// Session polls: the DM offers three dates as one poll, confirms one, and the others go.
// Players get one "suggested" and one "confirmed" notification for the whole poll.
const { test, expect, loginAs, apiAs, sojournId, goToMonth } = require('./helpers');
const env = require('./env');

// A day in the month three ahead: clear of the seed (+7, +14 days) and the other tests (+35..+49).
function dayInMonthAhead(day) {
    const now = new Date();
    const d = new Date(now.getFullYear(), now.getMonth() + 3, day);
    const p = (v) => String(v).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
const longDay = (ymd) => new Date(ymd + 'T12:00:00').toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' });
const shortDay = (ymd) => new Date(ymd + 'T12:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });

test('a poll of three dates: confirm one, the others go, one notification each', async ({ page }) => {
    const admin = await apiAs(env.ADMIN_USERNAME);
    const testy = await apiAs('testy');
    const id = await sojournId(admin);
    const [d1, d2, d3] = [dayInMonthAhead(10), dayInMonthAhead(12), dayInMonthAhead(16)];
    let confirmedId = '';

    try {
        // The DM has no unanswered suggestion, so open the calendar directly (not openCalendar).
        await loginAs(page, env.ADMIN_USERNAME);
        const monthLoaded = page.waitForResponse(/\/sessions\?month=/);
        await page.goto(`/games/${id}/calendar`);
        await monthLoaded;
        await goToMonth(page, d1);
        const cal = page.locator('#calendarPage');

        // Pick three days, then set the times in the dialog.
        await cal.getByRole('button', { name: 'Suggest dates' }).click();
        for (const d of [d1, d2, d3]) {
            await cal.getByRole('button', { name: new RegExp(`^Add ${longDay(d)} to the poll`) }).click();
        }
        await cal.getByRole('button', { name: 'Next: set times (3)' }).click();
        const dialog = page.getByRole('dialog', { name: 'Suggest dates' });
        await dialog.getByLabel(`Start on ${shortDay(d1)}`).fill('19:00');
        await dialog.getByLabel(`Length on ${shortDay(d1)}`).selectOption('240');
        await dialog.getByRole('button', { name: "Use the first row's time for all" }).click();
        const posted = page.waitForResponse((r) => /\/polls$/.test(r.url()) && r.request().method() === 'POST');
        await dialog.getByRole('button', { name: 'Suggest 3 dates' }).click();
        const poll = await (await posted).json();
        expect(poll.sessions).toHaveLength(3);
        const byDate = Object.fromEntries(poll.sessions.map((s) => [s.date, s]));
        confirmedId = byDate[d2].id;

        // The month marks the three dates as one poll.
        await expect(cal.locator('.day-poll-tag')).toHaveCount(3);
        await expect(cal.getByRole('button', { name: /Suggested 19:00-23:00.*Poll A, date 2 of 3/ })).toBeVisible();

        // A player answers every date.
        for (const s of poll.sessions) {
            const r = await testy.post(`/api/campaigns/${id}/sessions/${s.id}/responses`, { data: { participation: 'Accepted' } });
            expect(r.status()).toBe(201);
        }

        // The DM confirms the middle date; the warning names the dates that go.
        await cal.getByRole('button', { name: /Suggested 19:00-23:00.*date 2 of 3/ }).click();
        const block = cal.locator('.panel-session').filter({ hasText: shortDay(d2) });
        await block.getByRole('button', { name: 'Confirm' }).click();
        await expect(cal.getByText(`Confirm ${shortDay(d2)}? ${shortDay(d1)} and ${shortDay(d3)} will be removed.`)).toBeVisible();
        await block.getByRole('button', { name: 'Confirm' }).click();

        await expect(cal.getByRole('button', { name: /Confirmed 19:00-23:00/ })).toBeVisible();
        await expect(cal.locator('.day-poll-tag')).toHaveCount(0);
        await expect(cal.getByRole('button', { name: new RegExp(`^${longDay(d1)}.*Suggested`) })).toHaveCount(0);
        await expect(cal.getByRole('button', { name: new RegExp(`^${longDay(d3)}.*Suggested`) })).toHaveCount(0);

        const month = d1.slice(0, 7);
        const sessions = await (await admin.get(`/api/campaigns/${id}/sessions?month=${month}`)).json();
        const onPollDays = sessions.filter((s) => [d1, d2, d3].includes(s.date));
        expect(onPollDays.map((s) => [s.date, s.status])).toEqual([[d2, 'Confirmed']]);

        // One "suggested" and one "confirmed" for the player, nothing for the removed dates.
        const inbox = await (await testy.get('/api/notifications?limit=50')).json();
        const list = Array.isArray(inbox) ? inbox : inbox.notifications;
        const removed = [byDate[d1].id, byDate[d3].id];
        expect(list.filter((n) => n.type === 'session_suggested' && n.data?.poll_id === poll.poll_id)).toHaveLength(1);
        expect(list.filter((n) => n.type === 'session_confirmed' && n.session_id === confirmedId)).toHaveLength(1);
        expect(list.filter((n) => n.type === 'session_cancelled' && (n.data?.poll_id === poll.poll_id || removed.includes(n.session_id)))).toHaveLength(0);
    } finally {
        if (confirmedId) {
            await admin.put(`/api/campaigns/${id}/sessions/${confirmedId}`, { data: { status: 'Cancelled' } });
        }
        await testy.dispose();
        await admin.dispose();
    }
});
