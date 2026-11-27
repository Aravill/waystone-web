// Jukebox pop-up (/games/{id}/jukebox): the DM's broadcaster view and the listeners' player.
// Design: docs/design/jukebox.md. The DM's browser records audio with MediaRecorder (webm/opus) and sends the
// chunks over a WebSocket; listeners play what the server relays through Media Source Extensions.
(function () {
    const MSE_TYPE = 'audio/webm; codecs="opus"';
    const REC_TYPE = 'audio/webm;codecs=opus';
    const VOLUME_KEY = 'waystone-jukebox-volume';
    const DM_BACKOFF = [1, 2, 4, 8, 15];
    const LISTEN_BACKOFF = [1, 2, 3, 5, 10];

    const ENDED_TEXT = {
        taken_over: 'Broadcasting from another window. This one has stopped.',
        superseded: 'The jukebox was opened in another window.',
        no_access: 'You no longer have access to this jukebox',
        not_ongoing: 'This game is no longer ongoing, so the jukebox has ended.',
        deleted: 'This game was deleted, so the jukebox has ended.',
        shared_ended: 'The shared audio stopped, so the broadcast has ended.',
    };

    function loadVolume() {
        try {
            const v = JSON.parse(localStorage.getItem(VOLUME_KEY) || 'null');
            if (v && typeof v.volume === 'number') return { volume: Math.min(1, Math.max(0, v.volume)), muted: !!v.muted };
        } catch (error) { /* storage blocked or bad value: use the defaults */ }
        return { volume: 0.8, muted: false };
    }

    function saveVolume(volume, muted) {
        try { localStorage.setItem(VOLUME_KEY, JSON.stringify({ volume, muted })); } catch (error) { /* not saved */ }
    }

    window.jukeboxPopup = function () {
        // Objects the browser owns (sockets, recorders, media sources) live here, outside Alpine's proxies.
        const rt = {
            ws: null, retry: 0, failures: 0, ended: false,
            // DM capture graph
            tracks: [], ctx: null, gain: null, analyser: null, dest: null, rec: null, raf: 0,
            // Listener media
            audio: null, ms: null, sb: null, url: '', queue: [], first: true,
            bc: null, instance: Math.random().toString(36).slice(2) + Date.now().toString(36), openedAt: Date.now(),
        };

        return {
            gameId: '',
            me: '',
            title: '',
            role: '', // 'dm' | 'listener'
            view: 'loading', // loading | notfound | unsupported | start | live | listen | ended
            unsupportedText: '',
            endedReason: '',
            messageText: '', messageType: 'error', messageVisible: false, messageTimer: null,

            // who is listening (from the server)
            dmUser: null,
            listeners: [],

            // DM
            canTab: false,
            source: 'input',
            devices: [],
            devicesUnnamed: false,
            deviceId: '',
            starting: false,
            startError: '',
            muted: false,
            sendVolume: 0.8,
            reconnecting: false,

            // Listener
            state: 'off', // live | paused | off
            volume: 0.8,
            listenMuted: false,
            needPlay: false,

            async init() {
                this.gameId = decodeURIComponent(location.pathname.split('/')[2] || '');
                try {
                    const userResponse = await fetch('/auth/current-user');
                    if (!userResponse.ok) {
                        window.location.href = '/login.html';
                        return;
                    }
                    const user = await userResponse.json();
                    this.me = user.user_id || user.id || '';
                } catch (error) {
                    console.error('Auth check failed:', error);
                    window.location.href = '/login.html';
                    return;
                }

                let campaign = null;
                try {
                    const response = await fetch('/api/campaigns/' + encodeURIComponent(this.gameId));
                    if (response.ok) campaign = await response.json();
                } catch (error) {
                    console.error('Failed to load the game:', error);
                    this.showMessage('Could not load the game', 'error');
                }
                const role = campaign && campaign.status === 'Ongoing' ? this.roleOf(campaign) : '';
                if (!role) {
                    this.view = 'notfound';
                    return;
                }
                this.role = role;
                this.title = campaign.title || 'Game';
                document.title = 'Waystone - Jukebox: ' + this.title;

                const saved = loadVolume();
                this.volume = saved.volume;
                this.listenMuted = saved.muted;

                this.startChannel();
                window.addEventListener('pagehide', () => this.leave());

                if (role === 'dm') this.initDM();
                else this.initListener();
            },

            roleOf(campaign) {
                if (campaign.dm === this.me) return 'dm';
                if ((campaign.players || []).includes(this.me) || campaign.is_following) return 'listener';
                return '';
            },

            wsUrl() {
                return (location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + location.host +
                    '/api/campaigns/' + encodeURIComponent(this.gameId) + '/jukebox/ws?role=' + (this.role === 'dm' ? 'dm' : 'listen');
            },

            showMessage(text, type) {
                this.messageText = text;
                this.messageType = type;
                this.messageVisible = true;
                clearTimeout(this.messageTimer);
                this.messageTimer = setTimeout(() => { this.messageVisible = false; }, 5000);
            },

            // After the connection fails: is the game gone, over, or are we out? '' means keep trying.
            async recheckAccess() {
                try {
                    const response = await fetch('/api/campaigns/' + encodeURIComponent(this.gameId));
                    if (response.status === 404) return 'deleted';
                    if (response.status === 401) { window.location.href = '/login.html'; return ''; }
                    if (!response.ok) return '';
                    const campaign = await response.json();
                    if (campaign.status !== 'Ongoing') return 'not_ongoing';
                    if (this.roleOf(campaign) !== this.role) return 'no_access';
                } catch (error) { /* offline: keep trying */ }
                return '';
            },

            // The jukebox is over for good: stop everything and say why.
            endWith(reason) {
                rt.ended = true;
                clearTimeout(rt.retry);
                const ws = rt.ws;
                rt.ws = null;
                if (ws) { try { ws.close(1000); } catch (error) { /* closed */ } }
                this.teardownCapture();
                this.teardownMedia();
                this.reconnecting = false;
                this.endedReason = reason;
                this.dmUser = null;
                this.listeners = [];
                this.view = 'ended';
                this.announce();
            },

            get endedText() {
                return ENDED_TEXT[this.endedReason] || 'The jukebox has ended.';
            },

            handleSocketText(ws, raw) {
                let msg;
                try { msg = JSON.parse(raw); } catch (error) { return false; }
                if (msg.type === 'listeners') {
                    this.dmUser = msg.dm || null;
                    this.listeners = msg.listeners || [];
                } else if (msg.type === 'ended') {
                    this.endWith(msg.reason || '');
                } else {
                    return msg;
                }
                return true;
            },

            // ---------------------------------------------------------------- DM

            initDM() {
                const coarse = window.matchMedia && window.matchMedia('(pointer: coarse) and (hover: none)').matches;
                if (coarse || !window.MediaRecorder || !navigator.mediaDevices || !window.AudioContext) {
                    this.view = 'unsupported';
                    this.unsupportedText = coarse
                        ? 'Broadcasting needs a desktop browser. You can still start the jukebox from your computer.'
                        : "Broadcasting isn't supported in this browser.";
                    return;
                }
                this.canTab = !/Firefox\//.test(navigator.userAgent) && !!navigator.mediaDevices.getDisplayMedia;
                this.source = this.canTab ? 'tab' : 'input';
                this.view = 'start';
                this.loadDevices();
                this.$watch('source', (v) => { if (v === 'input') this.loadDevices(); });
            },

            async loadDevices() {
                try {
                    const all = await navigator.mediaDevices.enumerateDevices();
                    this.devices = all.filter((d) => d.kind === 'audioinput' && d.deviceId && d.deviceId !== 'default' && d.deviceId !== 'communications');
                    this.devicesUnnamed = this.devices.length === 0 || this.devices.some((d) => !d.label);
                } catch (error) {
                    console.error('Failed to list audio inputs:', error);
                }
            },

            // Device names stay blank until the browser has been allowed to use an input once.
            async unlockDevices() {
                try {
                    const s = await navigator.mediaDevices.getUserMedia({ audio: true });
                    s.getTracks().forEach((t) => t.stop());
                    await this.loadDevices();
                } catch (error) {
                    this.showMessage('The browser did not allow access to your audio inputs', 'error');
                }
            },

            async capture() {
                const raw = { echoCancellation: false, noiseSuppression: false, autoGainControl: false };
                if (this.source === 'tab') {
                    const stream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: raw, systemAudio: 'include' });
                    stream.getVideoTracks().forEach((t) => t.stop());
                    if (stream.getAudioTracks().length === 0) {
                        stream.getTracks().forEach((t) => t.stop());
                        throw new Error('No audio was shared. Tick "Share tab audio" (or "Share system audio") in the picker.');
                    }
                    return stream;
                }
                const audio = this.deviceId ? { ...raw, deviceId: { exact: this.deviceId } } : raw;
                return navigator.mediaDevices.getUserMedia({ audio });
            },

            async start() {
                if (this.starting) return;
                this.starting = true;
                this.startError = '';
                try {
                    const stream = await this.capture();
                    const ctx = new AudioContext();
                    await ctx.resume();
                    const gain = ctx.createGain();
                    gain.gain.value = this.muted ? 0 : this.sendVolume;
                    const analyser = ctx.createAnalyser();
                    analyser.fftSize = 1024;
                    const dest = ctx.createMediaStreamDestination();
                    ctx.createMediaStreamSource(stream).connect(gain);
                    gain.connect(analyser);
                    gain.connect(dest);
                    rt.tracks = stream.getAudioTracks();
                    rt.tracks.forEach((t) => { t.onended = () => { if (rt.ctx === ctx) this.endWith('shared_ended'); }; });
                    Object.assign(rt, { ctx, gain, analyser, dest });
                    rt.ended = false;
                    rt.failures = 0;
                    this.view = 'live';
                    this.$nextTick(() => this.drawMeter());
                    this.connectDM();
                    this.announce();
                } catch (error) {
                    console.error('Failed to start the jukebox:', error);
                    this.teardownCapture();
                    const name = error && error.name;
                    this.startError = name === 'NotAllowedError' ? 'The browser did not allow it. Allow access to start.'
                        : name === 'NotFoundError' ? 'No audio input was found.'
                        : (error && error.message) || 'Could not start the jukebox.';
                } finally {
                    this.starting = false;
                }
            },

            drawMeter() {
                const data = new Uint8Array(1024);
                const tick = () => {
                    if (!rt.analyser) return;
                    rt.analyser.getByteTimeDomainData(data);
                    let peak = 0;
                    for (let i = 0; i < data.length; i++) peak = Math.max(peak, Math.abs(data[i] - 128));
                    const bar = this.$refs.meter;
                    if (bar) bar.style.width = Math.min(100, Math.round((peak / 128) * 100)) + '%';
                    rt.raf = requestAnimationFrame(tick);
                };
                cancelAnimationFrame(rt.raf);
                tick();
            },

            connectDM() {
                clearTimeout(rt.retry);
                const ws = new WebSocket(this.wsUrl());
                rt.ws = ws;
                let opened = false;
                ws.onopen = () => {
                    if (rt.ws !== ws) return;
                    opened = true;
                    rt.failures = 0;
                    this.reconnecting = false;
                    ws.send(JSON.stringify({ muted: this.muted }));
                    this.startRecorder(ws); // a new recorder means a new init segment, which the server needs on every connection
                    this.announce();
                };
                ws.onmessage = (event) => {
                    if (rt.ws !== ws || typeof event.data !== 'string') return;
                    this.handleSocketText(ws, event.data);
                };
                ws.onclose = () => {
                    if (rt.ws !== ws) return;
                    this.stopRecorder();
                    if (rt.ended) return;
                    if (!opened) rt.failures++;
                    this.reconnecting = true;
                    this.announce();
                    const delay = DM_BACKOFF[Math.min(rt.failures, DM_BACKOFF.length - 1)];
                    rt.retry = setTimeout(async () => {
                        if (rt.ended || rt.ws !== ws) return;
                        if (rt.failures > 0) {
                            const reason = await this.recheckAccess();
                            if (reason) { this.endWith(reason); return; }
                        }
                        if (!rt.ended && rt.ws === ws) this.connectDM();
                    }, delay * 1000);
                };
            },

            startRecorder(ws) {
                this.stopRecorder();
                if (!rt.dest) return;
                const options = { audioBitsPerSecond: 128000 };
                if (MediaRecorder.isTypeSupported(REC_TYPE)) options.mimeType = REC_TYPE;
                let rec;
                try {
                    rec = new MediaRecorder(rt.dest.stream, options);
                } catch (error) {
                    console.error('MediaRecorder failed:', error);
                    this.showMessage("This browser can't record audio in the needed format", 'error');
                    return;
                }
                rec.ondataavailable = (event) => {
                    if (rt.ws !== ws || ws.readyState !== WebSocket.OPEN || !event.data || event.data.size === 0) return;
                    if (ws.bufferedAmount > 1048576) { ws.close(); return; } // the network can't keep up: start over
                    ws.send(event.data);
                };
                rec.start(250);
                rt.rec = rec;
            },

            stopRecorder() {
                const rec = rt.rec;
                rt.rec = null;
                if (!rec) return;
                rec.ondataavailable = null;
                try { if (rec.state !== 'inactive') rec.stop(); } catch (error) { /* already stopped */ }
            },

            teardownCapture() {
                this.stopRecorder();
                cancelAnimationFrame(rt.raf);
                rt.tracks.forEach((t) => { t.onended = null; t.stop(); });
                rt.tracks = [];
                if (rt.ctx) rt.ctx.close().catch(() => {});
                Object.assign(rt, { ctx: null, gain: null, analyser: null, dest: null });
            },

            applySendVolume() {
                if (rt.gain && !this.muted) rt.gain.gain.value = this.sendVolume;
                this.announce();
            },

            toggleMute() {
                this.setMuted(!this.muted);
            },

            setMuted(value) {
                this.muted = !!value;
                if (rt.gain) rt.gain.gain.value = this.muted ? 0 : this.sendVolume;
                if (rt.ws && rt.ws.readyState === WebSocket.OPEN) rt.ws.send(JSON.stringify({ muted: this.muted }));
                this.announce();
            },

            // Stop: end the broadcast and go back to choosing a source.
            stopBroadcast() {
                clearTimeout(rt.retry);
                const ws = rt.ws;
                rt.ws = null;
                if (ws) { try { ws.close(1000); } catch (error) { /* closed */ } }
                this.teardownCapture();
                this.reconnecting = false;
                this.muted = false;
                this.dmUser = null;
                this.listeners = [];
                this.view = 'start';
                this.loadDevices();
                this.announce();
            },

            get dmStatusKey() {
                return this.reconnecting ? 'off' : this.muted ? 'paused' : 'live';
            },
            get dmStatusGlyph() {
                return { live: '●', paused: '◐', off: '○' }[this.dmStatusKey];
            },
            get dmStatusText() {
                return this.reconnecting ? 'Reconnecting…' : this.muted ? 'Paused' : 'Live';
            },

            // ---------------------------------------------------------------- Listener

            initListener() {
                if (!window.MediaSource || !MediaSource.isTypeSupported(MSE_TYPE)) {
                    this.view = 'unsupported';
                    this.unsupportedText = "Listening isn't supported in this browser.";
                    return;
                }
                rt.audio = new Audio();
                rt.audio.preload = 'auto';
                rt.audio.volume = this.volume;
                rt.audio.muted = this.listenMuted;
                rt.audio.addEventListener('playing', () => { this.needPlay = false; });
                rt.audio.addEventListener('timeupdate', () => this.skipToLive());
                this.view = 'listen';
                this.connectListener();
            },

            get stateGlyph() {
                return { live: '●', paused: '◐', off: '○' }[this.state];
            },
            get stateText() {
                return { live: 'Live', paused: 'Paused', off: 'Off air — waiting for the DM' }[this.state];
            },

            connectListener() {
                clearTimeout(rt.retry);
                const ws = new WebSocket(this.wsUrl());
                ws.binaryType = 'arraybuffer';
                rt.ws = ws;
                let opened = false;
                ws.onopen = () => {
                    if (rt.ws !== ws) return;
                    opened = true;
                    rt.failures = 0;
                };
                ws.onmessage = (event) => {
                    if (rt.ws !== ws) return;
                    if (typeof event.data !== 'string') {
                        this.enqueue(event.data);
                        return;
                    }
                    const msg = this.handleSocketText(ws, event.data);
                    if (!msg || msg === true) return;
                    if (msg.type === 'stream') this.resetMedia();
                    else if (msg.type === 'state') this.state = msg.state === 'live' || msg.state === 'paused' ? msg.state : 'off';
                    this.announce();
                };
                ws.onclose = () => {
                    if (rt.ws !== ws || rt.ended) return;
                    this.state = 'off';
                    this.dmUser = null;
                    this.announce();
                    if (!opened) rt.failures++;
                    const delay = LISTEN_BACKOFF[Math.min(rt.failures, LISTEN_BACKOFF.length - 1)];
                    rt.retry = setTimeout(async () => {
                        if (rt.ended || rt.ws !== ws) return;
                        if (rt.failures > 0) {
                            const reason = await this.recheckAccess();
                            if (reason) { this.endWith(reason); return; }
                        }
                        if (!rt.ended && rt.ws === ws) this.connectListener();
                    }, delay * 1000);
                };
            },

            // A new stream (first join, or the DM restarted): throw the old source away and build a fresh one.
            resetMedia() {
                this.teardownMedia();
                const ms = new MediaSource();
                rt.ms = ms;
                rt.first = true;
                rt.url = URL.createObjectURL(ms);
                rt.audio.src = rt.url;
                ms.addEventListener('sourceopen', () => {
                    if (rt.ms !== ms || rt.sb) return;
                    try {
                        const sb = ms.addSourceBuffer(MSE_TYPE);
                        rt.sb = sb;
                        sb.addEventListener('updateend', () => this.afterUpdate());
                        sb.addEventListener('error', () => this.mediaFailed('source buffer error'));
                        this.pump();
                    } catch (error) {
                        this.mediaFailed(error);
                    }
                });
                this.tryPlay();
            },

            teardownMedia() {
                rt.queue = [];
                rt.sb = null;
                rt.ms = null;
                if (rt.audio) {
                    rt.audio.pause();
                    rt.audio.removeAttribute('src');
                    rt.audio.load();
                }
                if (rt.url) URL.revokeObjectURL(rt.url);
                rt.url = '';
            },

            // The media pipeline broke: reconnect to get a fresh init segment.
            mediaFailed(error) {
                console.error('Jukebox playback failed:', error);
                const ws = rt.ws;
                if (ws && ws.readyState <= WebSocket.OPEN) ws.close();
            },

            enqueue(buffer) {
                rt.queue.push(buffer);
                this.pump();
            },

            pump() {
                const sb = rt.sb;
                if (!sb || sb.updating || !rt.ms || rt.ms.readyState !== 'open') return;
                const buffered = rt.audio.buffered;
                // Drop what has been played long ago.
                if (buffered.length > 0 && rt.audio.currentTime - buffered.start(0) > 30) {
                    try { sb.remove(buffered.start(0), rt.audio.currentTime - 10); return; } catch (error) { /* try again later */ }
                }
                const chunk = rt.queue.shift();
                if (!chunk) return;
                try {
                    sb.appendBuffer(chunk);
                } catch (error) {
                    if (error && error.name === 'QuotaExceededError' && buffered.length > 0 && rt.audio.currentTime - buffered.start(0) > 1) {
                        rt.queue.unshift(chunk);
                        try { sb.remove(buffered.start(0), rt.audio.currentTime - 1); } catch (e) { this.mediaFailed(e); }
                    } else {
                        this.mediaFailed(error);
                    }
                }
            },

            afterUpdate() {
                const audio = rt.audio;
                if (rt.first && audio.buffered.length > 0) {
                    // A late joiner's timestamps start at the live point, not at zero: jump to the start of what we have.
                    rt.first = false;
                    audio.currentTime = audio.buffered.start(0);
                    this.tryPlay();
                }
                this.skipToLive();
                this.pump();
            },

            // More than 3 seconds behind the newest data: skip ahead.
            skipToLive() {
                const audio = rt.audio;
                if (!audio || rt.first || audio.buffered.length === 0) return;
                const end = audio.buffered.end(audio.buffered.length - 1);
                if (end - audio.currentTime > 3) audio.currentTime = end - 0.5;
            },

            tryPlay() {
                if (!rt.audio) return;
                const p = rt.audio.play();
                if (p && p.catch) {
                    p.then(() => { this.needPlay = false; }).catch((error) => {
                        // An abort from a source swap is not a block: only autoplay refusals need the button.
                        if (error && error.name === 'NotAllowedError') this.needPlay = true;
                    });
                }
            },

            playNow() {
                this.needPlay = false;
                this.tryPlay();
            },

            applyVolume() {
                if (rt.audio) rt.audio.volume = this.volume;
                saveVolume(this.volume, this.listenMuted);
                this.announce();
            },

            toggleListenMute() {
                this.setListenMuted(!this.listenMuted);
            },

            setListenMuted(value) {
                this.listenMuted = !!value;
                if (rt.audio) rt.audio.muted = this.listenMuted;
                saveVolume(this.volume, this.listenMuted);
                this.announce();
            },

            // ---------------------------------------------------------------- Header remote (BroadcastChannel)

            snapshot() {
                const dm = this.role === 'dm';
                return {
                    type: 'hello',
                    instance: rt.instance,
                    opened_at: rt.openedAt,
                    campaign_id: this.gameId,
                    title: this.title,
                    role: this.role,
                    state: dm ? (this.view === 'live' ? this.dmStatusKey : 'off') : this.state,
                    broadcasting: dm && this.view === 'live',
                    volume: dm ? this.sendVolume : this.volume,
                    muted: dm ? this.muted : this.listenMuted,
                };
            },

            announce() {
                if (rt.bc && this.role) {
                    try { rt.bc.postMessage(this.snapshot()); } catch (error) { /* channel closed */ }
                }
            },

            startChannel() {
                if (!window.BroadcastChannel) return;
                const bc = new BroadcastChannel('waystone-jukebox');
                rt.bc = bc;
                bc.onmessage = (event) => {
                    const m = event.data || {};
                    if (m.type === 'hello' && m.instance !== rt.instance) {
                        if (m.opened_at > rt.openedAt) this.supersede(); // the newest pop-up wins
                    } else if (m.type === 'ping') {
                        this.announce();
                    } else if (m.campaign_id === this.gameId) {
                        if (m.type === 'volume') {
                            const v = Math.min(1, Math.max(0, Number(m.value)));
                            if (Number.isNaN(v)) return;
                            if (this.role === 'dm') { this.sendVolume = v; this.applySendVolume(); }
                            else { this.volume = v; this.applyVolume(); }
                        } else if (m.type === 'mute') {
                            if (this.role === 'dm') { if (this.view === 'live') this.setMuted(!!m.value); }
                            else this.setListenMuted(!!m.value);
                        } else if (m.type === 'stop') {
                            if (this.role === 'dm') { if (this.view === 'live') this.stopBroadcast(); }
                            else window.close();
                        }
                    } else if (m.type === 'switch' && m.campaign_id && this.role !== 'dm') {
                        location.replace('/games/' + encodeURIComponent(m.campaign_id) + '/jukebox');
                    }
                };
                this.$watch(() => JSON.stringify(this.snapshot()), () => this.announce());
                this.announce();
            },

            // Another pop-up opened after this one: stop here and close if the browser lets us.
            supersede() {
                if (this.view === 'ended' || this.view === 'notfound' || this.view === 'unsupported') return;
                this.endWith('superseded');
                window.close();
            },

            // The window is going away (closed, or navigated to another game).
            leave() {
                rt.ended = true;
                clearTimeout(rt.retry);
                const ws = rt.ws;
                rt.ws = null;
                if (ws) { try { ws.close(1000); } catch (error) { /* closed */ } }
                if (rt.bc && this.role) {
                    try { rt.bc.postMessage({ type: 'bye', instance: rt.instance }); } catch (error) { /* closed */ }
                }
            },
        };
    };
})();
