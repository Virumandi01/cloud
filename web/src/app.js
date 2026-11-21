let currentUser = null;
let currentSubpath = '';

window.addEventListener('DOMContentLoaded', () => {
  setupDragAndDrop();
  refreshSession();
});

async function refreshSession() {
  const res = await fetch('/api/auth/me');
  if (!res.ok) {
    document.getElementById('loginModal').classList.add('open');
    return;
  }
  currentUser = await res.json();
  document.getElementById('loginModal').classList.remove('open');
  document.getElementById('userLabel').textContent = currentUser.username;

  const usedGB = (currentUser.used_bytes / 1e9).toFixed(2);
  const limitGB = (currentUser.storage_limit_bytes / 1e9).toFixed(1);
  document.getElementById('quotaText').textContent = `${usedGB} / ${limitGB} GB`;
  const pct = Math.min(100, (currentUser.used_bytes / currentUser.storage_limit_bytes) * 100);
  document.getElementById('quotaFill').style.width = `${pct}%`;

  // Show Admin Console only for host admin
  document.getElementById('navAdmin').style.display = currentUser.role === 'admin' ? 'flex' : 'none';

  const wp = document.getElementById('wallpaper');
  wp.style.display = 'block';
  wp.src = `/api/user/wallpaper?t=${Date.now()}`;

  loadFiles(currentSubpath);
}

async function submitLogin() {
  const username = document.getElementById('loginUser').value.trim();
  const passcode = document.getElementById('loginPass').value;
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, passcode })
  });
  if (!res.ok) {
    document.getElementById('loginError').textContent = 'Invalid username or passcode.';
    return;
  }
  document.getElementById('loginError').textContent = '';
  refreshSession();
}

async function logout() {
  await fetch('/api/auth/logout', { method: 'POST' });
  currentUser = null;
  document.getElementById('loginModal').classList.add('open');
}

function switchTab(tab) {
  ['files', 'audio', 'profile', 'admin'].forEach(t => {
    document.getElementById(`tab${t[0].toUpperCase() + t.slice(1)}`).style.display = t === tab ? 'block' : 'none';
    const nav = document.getElementById(`nav${t[0].toUpperCase() + t.slice(1)}`);
    if (nav) nav.classList.toggle('active', t === tab);
  });
  if (tab === 'audio') loadAudio('alpha');
  if (tab === 'admin') loadAdminUsers();
}

// Drag & Drop Anywhere
function setupDragAndDrop() {
  const zone = document.getElementById('dropZone');
  const overlay = document.getElementById('dropOverlay');
  ['dragenter', 'dragover'].forEach(evt => zone.addEventListener(evt, e => {
    e.preventDefault(); overlay.classList.add('active');
  }));
  ['dragleave', 'drop'].forEach(evt => zone.addEventListener(evt, e => {
    e.preventDefault(); overlay.classList.remove('active');
  }));
  zone.addEventListener('drop', e => {
    if (e.dataTransfer.files.length && currentUser) {
      handlePickedFiles(e.dataTransfer.files);
    }
  });
}

function handlePickedFiles(fileList) {
  for (const file of fileList) {
    uploadFileResumable(file, currentUser.username, currentSubpath);
  }
}

async function loadFiles(subpath = '') {
  currentSubpath = subpath;
  renderBreadcrumbs();
  const res = await fetch(`/api/files/list?user=${encodeURIComponent(currentUser.username)}&subpath=${encodeURIComponent(subpath)}`);
  if (!res.ok) return;
  const data = await res.json();
  const tbody = document.getElementById('fileListBody');
  tbody.innerHTML = '';

  for (const item of data.items) {
    const tr = document.createElement('tr');
    tr.className = 'file-row';
    const accessKey = `${currentUser.username}/${item.path}`;
    const sizeStr = item.is_dir ? 'Folder' : formatBytes(item.size);

    tr.innerHTML = `
      <td>
        <div class="file-name" onclick="${item.is_dir ? `loadFiles('${item.path}')` : `openViewer('${item.path}', '${item.ext}')`}">
          <span>${item.is_dir ? '📁' : '📄'}</span>
          <span>${item.name}</span>
        </div>
      </td>
      <td style="color:var(--text-muted);">${sizeStr}</td>
      <td>
        ${item.is_dir ? '—' : `<span class="badge ${item.is_public ? 'badge-public' : 'badge-private'}" onclick="togglePublic('${accessKey}')">
          ${item.is_public ? '🌐 Everyone (access.txt)' : '🔒 Private'}
        </span>`}
      </td>
      <td style="text-align:right;">
        ${item.is_dir ? '' : `<a class="btn" style="padding:4px 9px; font-size:0.8rem;" href="/api/files/download?user=${currentUser.username}&path=${encodeURIComponent(currentUser.username + '/' + item.path)}&download=true">↓</a>`}
        <button class="btn btn-danger" style="padding:4px 9px; font-size:0.8rem;" onclick="deleteFile('${item.path}')">✕</button>
      </td>
    `;
    tbody.appendChild(tr);
  }
}

function renderBreadcrumbs() {
  const bc = document.getElementById('breadcrumbs');
  const parts = currentSubpath ? currentSubpath.split('/') : [];
  let html = `<span class="crumb" onclick="loadFiles('')">Home</span>`;
  let acc = '';
  parts.forEach(p => {
    acc = acc ? `${acc}/${p}` : p;
    html += ` <span>/</span> <span class="crumb" onclick="loadFiles('${acc}')">${p}</span>`;
  });
  bc.innerHTML = html;
}

async function createNewFolder() {
  const name = prompt('Folder name:');
  if (!name) return;
  const full = currentSubpath ? `${currentSubpath}/${name}` : name;
  await fetch('/api/files/mkdir', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: currentUser.username, path: full })
  });
  loadFiles(currentSubpath);
}

async function togglePublic(accessKey) {
  await fetch('/api/access/toggle', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: accessKey })
  });
  loadFiles(currentSubpath);
}

async function deleteFile(relPath) {
  if (!confirm(`Delete ${relPath}?`)) return;
  await fetch('/api/files/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: currentUser.username, path: relPath })
  });
  loadFiles(currentSubpath);
  refreshSession();
}

// Universal File Viewer (PDF, Images, Video, Audio, Word, Excel, Zip, Text)
async function openViewer(relPath, ext) {
  const modal = document.getElementById('viewerModal');
  const body = document.getElementById('viewerBody');
  const fileUrl = `/api/files/download?user=${currentUser.username}&path=${encodeURIComponent(currentUser.username + '/' + relPath)}`;

  document.getElementById('viewerTitle').textContent = relPath;
  document.getElementById('viewerDownloadBtn').href = fileUrl + '&download=true';
  body.innerHTML = '<p style="color:var(--text-muted);">Loading preview...</p>';
  modal.classList.add('open');

  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(ext)) {
    body.innerHTML = `<img src="${fileUrl}" style="max-width:100%; max-height:100%; display:block; margin:auto;" />`;
  } else if (ext === 'pdf') {
    body.innerHTML = `<iframe src="${fileUrl}" style="width:100%; height:100%; border:none;"></iframe>`;
  } else if (['mp4', 'webm', 'mkv'].includes(ext)) {
    body.innerHTML = `<video src="${fileUrl}" controls autoplay style="width:100%; max-height:100%;"></video>`;
  } else if (['mp3', 'wav', 'flac', 'm4a', 'ogg'].includes(ext)) {
    body.innerHTML = `<audio src="${fileUrl}" controls autoplay style="width:100%; margin-top:40px;"></audio>`;
  } else if (ext === 'docx' && window.docx) {
    const blob = await (await fetch(fileUrl)).blob();
    body.innerHTML = '';
    await window.docx.renderAsync(blob, body);
  } else if (['xlsx', 'xls', 'csv'].includes(ext) && window.XLSX) {
    const buf = await (await fetch(fileUrl)).arrayBuffer();
    const wb = window.XLSX.read(buf);
    const html = window.XLSX.utils.sheet_to_html(wb.Sheets[wb.SheetNames[0]]);
    body.innerHTML = `<div style="background:#fff; color:#000; padding:12px; border-radius:8px; overflow:auto;">${html}</div>`;
  } else if (ext === 'zip') {
    const res = await fetch(`/api/archive/inspect?user=${currentUser.username}&path=${encodeURIComponent(relPath)}`);
    const data = await res.json();
    body.innerHTML = `<h4>Archive Contents:</h4><ul style="margin:12px 0 18px 20px;">` +
      (data.entries || []).map(e => `<li>${e.name} (${formatBytes(e.uncompressed_size)})</li>`).join('') +
      `</ul><button class="btn btn-primary" onclick="unzipHere('${relPath}')">Extract All Here</button>`;
  } else {
    const text = await (await fetch(fileUrl)).text();
    body.innerHTML = `<pre style="white-space:pre-wrap; font-family:monospace; font-size:0.9rem;">${text.replace(/</g, '&lt;')}</pre>`;
  }
}

async function unzipHere(relPath) {
  await fetch('/api/archive/unzip', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user: currentUser.username, path: relPath, dest: currentSubpath })
  });
  closeViewer();
  loadFiles(currentSubpath);
}

function closeViewer() {
  document.getElementById('viewerModal').classList.remove('open');
  document.getElementById('viewerBody').innerHTML = '';
}

async function uploadWallpaper(file) {
  if (!file) return;
  await fetch('/api/user/wallpaper', { method: 'POST', body: file });
  refreshSession();
}

// Audio Tab Loader
async function loadAudio(sort) {
  const res = await fetch(`/api/audio/library?user=${currentUser.username}&sort=${sort}`);
  const data = await res.json();
  const list = document.getElementById('audioList');
  list.innerHTML = (data.tracks || []).map(t => `
    <div class="file-row" style="display:flex; align-items:center; justify-content:space-between; padding:10px 12px; cursor:pointer;"
         onclick="playTrack('${t.id}', '${t.title.replace(/'/g, "\\'")}', '${t.artist.replace(/'/g, "\\'")}', '${t.path}', ${t.has_cover})">
      <div>
        <div style="font-weight:500;">${t.title}</div>
        <div style="font-size:0.82rem; color:var(--text-muted);">${t.artist} • ${t.album}</div>
      </div>
      <span style="color:var(--text-muted); font-size:0.82rem;">▶ Play</span>
    </div>
  `).join('');
}

function playTrack(id, title, artist, path, hasCover) {
  const bar = document.getElementById('audioPlayerBar');
  bar.style.display = 'flex';
  document.getElementById('nowTitle').textContent = title;
  document.getElementById('nowArtist').textContent = artist;
  const cover = document.getElementById('nowCover');
  cover.src = hasCover ? `/api/audio/cover?user=${currentUser.username}&id=${id}` : '';
  const audio = document.getElementById('htmlAudio');
  audio.src = `/api/files/download?user=${currentUser.username}&path=${encodeURIComponent(currentUser.username + '/' + path)}`;
  audio.play();
}

// Admin Console Functions
async function loadAdminUsers() {
  const res = await fetch('/api/admin/users');
  if (!res.ok) return;
  const users = await res.json();
  const tbody = document.getElementById('adminUsersBody');
  tbody.innerHTML = users.map(u => `
    <tr class="file-row">
      <td><strong>${u.username}</strong></td>
      <td>${u.role}</td>
      <td>${(u.used_bytes / 1e9).toFixed(2)} GB / ${(u.storage_limit_bytes / 1e9).toFixed(1)} GB</td>
      <td style="display:flex; gap:8px; align-items:center;">
        <input id="q_${u.username}" type="number" value="${Math.round(u.storage_limit_bytes / 1e9)}" style="width:80px; margin:0;" />
        <input id="p_${u.username}" type="password" placeholder="New passcode" style="width:140px; margin:0;" />
        <button class="btn btn-primary" onclick="adminSaveUser('${u.username}')">Save</button>
      </td>
    </tr>
  `).join('');
}

async function adminCreateUser() {
  const username = document.getElementById('admNewUser').value.trim();
  const passcode = document.getElementById('admNewPass').value;
  const gb = parseFloat(document.getElementById('admNewQuota').value) || 10;
  if (!username || !passcode) return alert('Enter username and passcode');

  const res = await fetch('/api/admin/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action: 'create', username, passcode, storage_limit_bytes: gb * 1e9 })
  });
  if (res.ok) {
    document.getElementById('admNewUser').value = '';
    document.getElementById('admNewPass').value = '';
    loadAdminUsers();
  }
}

async function adminSaveUser(username) {
  const gb = parseFloat(document.getElementById(`q_${username}`).value);
  const passcode = document.getElementById(`p_${username}`).value;
  await fetch('/api/admin/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action: 'update', username, passcode, storage_limit_bytes: gb * 1e9 })
  });
  alert(`Updated ${username}`);
  loadAdminUsers();
}

function formatBytes(b) {
  if (!b) return '0 B';
  const k = 1024, sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}