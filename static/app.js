let map, marker, locationConfirmed = false;

document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('alertForm');
  const getLocationBtn = document.getElementById('getLocation');
  const submitBtn = document.getElementById('submitBtn');

  getLocationBtn.addEventListener('click', getLocation);
  form.addEventListener('submit', submitAlert);

  // Enable submit when all required fields are filled
  form.querySelectorAll('input, textarea').forEach(el => {
    el.addEventListener('input', checkForm);
  });
});

function getLocation() {
  const btn = document.getElementById('getLocation');
  const status = document.getElementById('locationStatus');

  if (!navigator.geolocation) {
    status.innerHTML = '<span class="error">Tu dispositivo no soporta geolocalización</span>';
    return;
  }

  btn.classList.add('loading');
  btn.textContent = '📍 OBTENIENDO UBICACIÓN...';
  status.innerHTML = '<span>Obteniendo ubicación GPS...</span>';

  navigator.geolocation.getCurrentPosition(
    position => {
      const lat = position.coords.latitude;
      const lng = position.coords.longitude;
      document.getElementById('lat').value = lat;
      document.getElementById('lng').value = lng;

      btn.classList.remove('loading');
      btn.textContent = '📍 RELOCALIZAR';
      status.innerHTML = '<span class="success">✅ Ubicación obtenida</span>';

      showMap(lat, lng);
      locationConfirmed = true;
      checkForm();
    },
    error => {
      btn.classList.remove('loading');
      btn.textContent = '📍 OBTENER MI UBICACIÓN';
      let msg = 'Error al obtener ubicación. Asegúrate de tener el GPS activado.';
      if (error.code === 1) msg = 'Permiso denegado. Activa la ubicación en los ajustes.';
      else if (error.code === 2) msg = 'Señal GPS no disponible. Intenta en exteriores.';
      else if (error.code === 3) msg = 'Tiempo de espera agotado. Intenta de nuevo.';
      status.innerHTML = `<span class="error">❌ ${msg}</span>`;
      locationConfirmed = false;
      checkForm();
    },
    { enableHighAccuracy: true, timeout: 15000, maximumAge: 60000 }
  );
}

function showMap(lat, lng) {
  const mapContainer = document.getElementById('map');
  mapContainer.classList.add('visible');

  if (!map) {
    map = L.map('map', { zoomControl: false }).setView([lat, lng], 15);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap',
      maxZoom: 19
    }).addTo(map);
    marker = L.marker([lat, lng], { draggable: true }).addTo(map);
    marker.on('dragend', () => {
      const pos = marker.getLatLng();
      document.getElementById('lat').value = pos.lat;
      document.getElementById('lng').value = pos.lng;
    });
  } else {
    map.setView([lat, lng], 15);
    marker.setLatLng([lat, lng]);
  }

  setTimeout(() => map.invalidateSize(), 300);
}

function checkForm() {
  const name = document.getElementById('name').value.trim();
  const phone = document.getElementById('phone').value.trim();
  const desc = document.getElementById('description').value.trim();
  const btn = document.getElementById('submitBtn');

  btn.disabled = !(name && phone && desc && locationConfirmed);
}

async function submitAlert(e) {
  e.preventDefault();

  const btn = document.getElementById('submitBtn');
  btn.disabled = true;
  btn.classList.add('loading');
  btn.innerHTML = '<span class="btn-icon">⏳</span> ENVIANDO ALERTA...';

  const form = document.getElementById('alertForm');
  const formData = new FormData(form);

  try {
    const res = await fetch('/api/alert', { method: 'POST', body: formData });
    const data = await res.json();

    if (data.ok) {
      document.getElementById('alertId').textContent = data.id;
      document.getElementById('alertForm').classList.add('hidden');
      document.getElementById('confirmation').classList.remove('hidden');
    } else {
      alert('Error: ' + (data.error || 'No se pudo enviar la alerta'));
      btn.disabled = false;
      btn.classList.remove('loading');
      btn.innerHTML = '<span class="btn-icon">🆘</span> ENVIAR ALERTA SOS';
      checkForm();
    }
  } catch (err) {
    alert('Error de conexión. Verifica tu internet e intenta de nuevo.');
    btn.disabled = false;
    btn.classList.remove('loading');
    btn.innerHTML = '<span class="btn-icon">🆘</span> ENVIAR ALERTA SOS';
    checkForm();
  }
}

function resetForm() {
  document.getElementById('alertForm').reset();
  document.getElementById('alertForm').classList.remove('hidden');
  document.getElementById('confirmation').classList.add('hidden');
  document.getElementById('lat').value = '';
  document.getElementById('lng').value = '';
  locationConfirmed = false;

  if (map) { map.remove(); map = null; marker = null; }
  document.getElementById('map').classList.remove('visible');
  document.getElementById('locationStatus').innerHTML = '<span>Presiona el botón para obtener tu ubicación</span>';
  document.getElementById('getLocation').textContent = '📍 OBTENER MI UBICACIÓN';
  checkForm();
}
