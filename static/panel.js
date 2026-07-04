const PANEL_TOKEN_KEY = 'pinsos_panel_token';
let alerts = [];
let pollInterval = null;

document.addEventListener('DOMContentLoaded', () => {
  const filter = document.getElementById('statusFilter');
  const refreshBtn = document.getElementById('refreshBtn');

  // Check for saved token
  let token = localStorage.getItem(PANEL_TOKEN_KEY);
  if (!token) {
    token = prompt('Introduce el token del panel:');
    if (token) {
      localStorage.setItem(PANEL_TOKEN_KEY, token);
    } else {
      document.getElementById('alertsList').innerHTML = '<div class="loading">Token requerido para acceder al panel.</div>';
      return;
    }
  }

  filter.addEventListener('change', loadAlerts);
  refreshBtn.addEventListener('click', loadAlerts);

  loadAlerts();
  pollInterval = setInterval(loadAlerts, 30000);
});

async function loadAlerts() {
  const container = document.getElementById('alertsList');
  const filter = document.getElementById('statusFilter');
  const token = localStorage.getItem(PANEL_TOKEN_KEY);

  if (!token) return;

  try {
    const status = filter.value;
    let url = '/api/alerts';
    if (status) url += '?status=' + encodeURIComponent(status);

    const res = await fetch(url, {
      headers: { 'Authorization': 'Bearer ' + token }
    });

    if (res.status === 401) {
      localStorage.removeItem(PANEL_TOKEN_KEY);
      container.innerHTML = '<div class="loading">Token inválido. Recarga la página.</div>';
      return;
    }

    const data = await res.json();
    if (!data.ok) {
      container.innerHTML = '<div class="loading">Error al cargar alertas.</div>';
      return;
    }

    alerts = data.alerts || [];
    renderAlerts();
  } catch (err) {
    container.innerHTML = '<div class="loading">Error de conexión.</div>';
  }
}

function renderAlerts() {
  const container = document.getElementById('alertsList');
  const counter = document.getElementById('alertCounter');

  const activas = alerts.filter(a => a.status === 'ACTIVA').length;
  counter.textContent = activas + ' activas';

  if (alerts.length === 0) {
    container.innerHTML = '<div class="loading">No hay alertas</div>';
    return;
  }

  container.innerHTML = alerts.map(a => `
    <div class="alert-card status-${a.status}">
      <div class="alert-header">
        <span class="alert-id">#${a.id}</span>
        <span class="alert-time">${a.created_at}</span>
      </div>
      <div class="alert-name">${escapeHtml(a.name)}</div>
      <div class="alert-phone">📞 ${escapeHtml(a.phone)}</div>
      <div class="alert-desc">${escapeHtml(a.description)}</div>
      <div class="alert-meta">
        <a href="${a.maps_link}" target="_blank" rel="noopener">📍 Ver en Maps</a>
        <span class="status-badge">${a.status}</span>
      </div>
      ${a.photo_path ? `<img src="${a.photo_path}" class="alert-photo" alt="Foto alerta" onclick="window.open(this.src)">` : ''}
      ${a.status !== 'CERRADA' ? `
      <div class="alert-actions">
        ${a.status === 'ACTIVA' ? `<button class="btn-action atender" onclick="updateAlert(${a.id}, 'ATENDIDA')">✅ Atendida</button>` : ''}
        <button class="btn-action cerrar" onclick="updateAlert(${a.id}, 'CERRADA')">❌ Cerrar</button>
      </div>` : ''}
    </div>
  `).join('');
}

function escapeHtml(s) {
  if (!s) return '';
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

async function updateAlert(id, status) {
  const token = localStorage.getItem(PANEL_TOKEN_KEY);
  try {
    const res = await fetch('/api/alerts/' + id + '/status', {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer ' + token
      },
      body: JSON.stringify({ status })
    });
    const data = await res.json();
    if (data.ok) {
      loadAlerts();
    }
  } catch (err) {
    alert('Error al actualizar');
  }
}
