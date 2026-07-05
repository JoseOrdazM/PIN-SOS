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
  btn.innerHTML = '<span class="btn-icon">⏳</span> PREPARANDO...';

  const form = document.getElementById('alertForm');
  const formData = new FormData(form);

  // Comprimir foto antes de enviar si existe
  const photoInput = document.getElementById('photo');
  if (photoInput && photoInput.files && photoInput.files[0]) {
    btn.innerHTML = '<span class="btn-icon">⏳</span> COMPRIMIENDO FOTO...';
    try {
      const compressed = await compressImage(photoInput.files[0], 800, 0.7);
      formData.delete('photo');
      formData.append('photo', compressed, photoInput.files[0].name);
    } catch (err) {
      console.warn('Compresión falló, usando original:', err);
    }
  }

  btn.innerHTML = '<span class="btn-icon">⏳</span> ENVIANDO ALERTA...';

  // Usar xhr en vez de fetch para tener progreso
  try {
    const data = await uploadWithProgress(formData, btn);

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

function compressImage(file, maxDim, quality) {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      let w = img.width, h = img.height;
      if (w > maxDim || h > maxDim) {
        const ratio = Math.min(maxDim / w, maxDim / h);
        w = Math.round(w * ratio);
        h = Math.round(h * ratio);
      }
      const canvas = document.createElement('canvas');
      canvas.width = w;
      canvas.height = h;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(img, 0, 0, w, h);
      canvas.toBlob(blob => {
        if (blob) {
          const newFile = new File([blob], file.name, { type: 'image/jpeg' });
          resolve(newFile);
        } else {
          reject(new Error('Canvas toBlob failed'));
        }
      }, 'image/jpeg', quality);
    };
    img.onerror = () => reject(new Error('Image load failed'));
    img.src = URL.createObjectURL(file);
  });
}

function uploadWithProgress(formData, btnElement) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/alert', true);
    xhr.timeout = 60000;

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        const pct = Math.round((e.loaded / e.total) * 100);
        btnElement.innerHTML = `<span class="btn-icon">⏳</span> ENVIANDO... ${pct}%`;
      }
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch (e) {
          reject(new Error('Invalid response'));
        }
      } else {
        try {
          const err = JSON.parse(xhr.responseText);
          reject(new Error(err.error || 'Error del servidor'));
        } catch (e) {
          reject(new Error('Error del servidor (' + xhr.status + ')'));
        }
      }
    };

    xhr.onerror = () => reject(new Error('Error de conexión'));
    xhr.ontimeout = () => reject(new Error('Tiempo de espera agotado'));

    xhr.send(formData);
  });
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
