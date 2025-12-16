const GW_URL = 'http://localhost:4000/v1/chat';

// ⬇️ FILL THESE MANUALLY
const JWT = '';
const SESSION_ID = '';
const total = Number(1280);
const concurrency = Number(128);

async function post(i) {
    const rid = `burst-${Date.now()}-${i}-${Math.random().toString(16).slice(2)}`;

    const body = {
        message: `burst test ${rid}`,
        jobId: crypto.randomUUID(),
        sessionId: SESSION_ID,
        mode: 'gen',
    };

    const res = await fetch(GW_URL, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${JWT}`,
        },
        body: JSON.stringify(body),
    });
    return res.status;
}

async function run() {
    let idx = 0;
    const workers = Array.from({ length: concurrency }, async () => {
        while (idx < total) {
            const i = idx++;
            try { await post(i); } catch {}
        }
    });
    await Promise.all(workers);
    console.log('done', { total, concurrency });
}

run();