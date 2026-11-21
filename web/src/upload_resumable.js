const CHUNK_SIZE = 5 * 1024 * 1024; // 5MB chunks
let uploadPaused = false;

function toggleUploadPause() {
  uploadPaused = !uploadPaused;
  document.getElementById('txPauseBtn').textContent = uploadPaused ? 'Resume' : 'Pause';
  document.getElementById('txStatus').textContent = uploadPaused ? 'Paused by user' : 'Resuming transfer...';
}

async function uploadFileResumable(file, username, subpath) {
  const drawer = document.getElementById('transferDrawer');
  const bar = document.getElementById('txBar');
  const status = document.getElementById('txStatus');
  document.getElementById('txFileName').textContent = file.name;
  drawer.style.display = 'block';
  uploadPaused = false;

  const totalChunks = Math.max(1, Math.ceil(file.size / CHUNK_SIZE));
  const uploadID = crypto.randomUUID();
  const targetName = subpath ? `${subpath}/${file.name}` : file.name;

  for (let i = 0; i < totalChunks; i++) {
    while (uploadPaused) {
      await new Promise(r => setTimeout(r, 300));
    }

    const start = i * CHUNK_SIZE;
    const end = Math.min(file.size, start + CHUNK_SIZE);
    const chunk = file.slice(start, end);

    // Compute SHA-256 checksum on client before sending
    const buf = await chunk.arrayBuffer();
    const digest = await crypto.subtle.digest('SHA-256', buf);
    const shaHex = Array.from(new Uint8Array(digest)).map(b => b.toString(16).padStart(2, '0')).join('');

    let sent = false;
    while (!sent) {
      while (uploadPaused) await new Promise(r => setTimeout(r, 300));
      try {
        status.textContent = `Chunk ${i + 1}/${totalChunks} • SHA-256 verified`;
        const res = await fetch('/api/upload/chunk', {
          method: 'POST',
          headers: {
            'X-User': username,
            'X-Upload-ID': uploadID,
            'X-Chunk-Index': i.toString(),
            'X-Total-Chunks': totalChunks.toString(),
            'X-Chunk-SHA256': shaHex,
            'X-File-Name': targetName
          },
          body: chunk
        });

        if (res.status === 413) {
          status.textContent = 'Error: Storage quota exceeded!';
          return;
        }
        if (res.ok) {
          sent = true;
          const pct = Math.round(((i + 1) / totalChunks) * 100);
          bar.style.width = `${pct}%`;
        } else {
          throw new Error('Server rejected chunk');
        }
      } catch (err) {
        status.textContent = 'Offline or connection drop — retrying in 3s...';
        await new Promise(r => setTimeout(r, 3000));
      }
    }
  }

  status.textContent = 'Completed & verified!';
  setTimeout(() => { drawer.style.display = 'none'; bar.style.width = '0%'; }, 2000);
  loadFiles(currentSubpath);
  refreshSession();
}