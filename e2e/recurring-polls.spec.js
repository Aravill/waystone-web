// Recurring polls: the DM turns Repeat on for an open poll. With 21 days of lead time the next
// cycle is already due, so it is posted at once with every date a week later.
const { test, expect, loginAs, apiAs, sojournId, dateFromToday } = require('./helpers');
const env = require('./env');

const shortDay = (ymd) => new Date(ymd + 'T12:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });

test('repeat a poll: the next cycle is posted at once, and Stop ends it', async ({ page }) => {
    const admin = await apiAs(env.ADMIN_USERNAME);
    const testy = await apiAs('testy');
    const id = await sojournId(admin);
    // Clear of the seed (+7, +14) and the other tests (+35..+49, month 3 ahead)
    const [d1, d2] = [dateFromToday(10), dateFromToday(11)];
    const [n1, n2] = [dateFromToday(17), dateFromToday(18)];
    const pollIds = [];

    try {
        const created = await admin.post(`/api/campaigns/${id}/polls`, {
            data: {
                timezone: 'Europe/Prague',
                dates: [
                    { date: d1, time: '19:00', duration: 240 },
                    { date: d2, time: '14:00', duration: 300 },
                ],
            },
        });
        expect(created.status()).toBe(201);
        const source = (await created.json()).poll_id;
        pollIds.push(source);

        // Open the calendar on the poll's first day and turn Repeat on.
        await loginAs(page, env.ADMIN_USERNAME);
        await page.goto(`/games/${id}/calendar?date=${d1}`);
        const cal = page.locator('#calendarPage');
        await cal.getByRole('button', { name: 'Repeat…' }).click();
        const dialog = page.getByRole('dialog', { name: 'Repeat this poll' });
        await expect(dialog).toContainText('19:00–23:00');
        await expect(dialog).toContainText('14:00–19:00');
        await dialog.getByLabel('Post').selectOption('21');
        const saved = page.waitForResponse((r) => /poll-schedule$/.test(r.url()) && r.request().method() === 'PUT');
        await dialog.getByRole('button', { name: 'Repeat', exact: true }).click();
        expect((await saved).status()).toBe(200);
        await expect(dialog).toBeHidden();

        // The schedule exists, and its next cycle (2) is the one after the already posted one.
        const sc = (await (await admin.get(`/api/campaigns/${id}/poll-schedule`)).json()).schedule;
        expect(sc.every_weeks).toBe(1);
        expect(sc.lead_days).toBe(21);
        expect(sc.next_cycle).toBe(2);
        expect(sc.next_dates).toEqual([dateFromToday(24), dateFromToday(25)]);

        // A new poll holds every date shifted by 7 days; the source poll is untouched.
        const game = await (await admin.get(`/api/campaigns/${id}`)).json();
        const suggested = (game.upcoming_sessions || []).filter((s) => s.status === 'Suggested' && s.poll_id);
        const fresh = suggested.filter((s) => s.poll_id !== source && [n1, n2].includes(s.date));
        expect(fresh.map((s) => [s.date, s.time, s.duration]).sort()).toEqual([[n1, '19:00', 240], [n2, '14:00', 300]]);
        expect(new Set(fresh.map((s) => s.poll_id)).size).toBe(1);
        pollIds.push(fresh[0].poll_id);
        expect(suggested.filter((s) => s.poll_id === source)).toHaveLength(2);

        // The DM hears once; the player got the poll as an ordinary suggestion.
        const listOf = async (api) => {
            const body = await (await api.get('/api/notifications?limit=100')).json();
            return Array.isArray(body) ? body : body.notifications;
        };
        const posted = (await listOf(admin)).filter((n) => n.type === 'poll_posted' && n.data?.poll_id === fresh[0].poll_id);
        expect(posted).toHaveLength(1);
        expect(posted[0].data.dates).toBe(`${n1},${n2}`);
        const suggestions = (await listOf(testy)).filter((n) => n.type === 'session_suggested' && n.data?.poll_id === fresh[0].poll_id);
        expect(suggestions).toHaveLength(1);

        // The game page shows the line; Stop (after asking) removes the schedule.
        await page.goto(`/games/${id}`);
        const line = page.locator('.game-repeat');
        await expect(line).toContainText('New poll every week: ');
        await expect(line).toContainText('Next posted ' + shortDay(dateFromToday(24 - 21)));
        await line.getByRole('button', { name: 'Stop' }).click();
        await page.getByRole('dialog', { name: 'Stop repeating' }).getByRole('button', { name: 'Stop repeating' }).click();
        await expect(line).toBeHidden();
        expect((await (await admin.get(`/api/campaigns/${id}/poll-schedule`)).json()).schedule).toBeNull();
    } finally {
        await admin.delete(`/api/campaigns/${id}/poll-schedule`);
        for (const pollId of pollIds) await admin.delete(`/api/campaigns/${id}/polls/${pollId}`);
        await testy.dispose();
        await admin.dispose();
    }
});
