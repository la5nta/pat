import { alert, isInsecureOrigin, dateFormat } from '../utils/index.js';

let posId = 0;

function handleGeolocationError(error, $container, statusPopoverInstance) {
  let message = 'Geolocation error.';
  if (error && error.message) {
    message = error.message;
  }
  if (error.message.search('insecure origin') > 0 || isInsecureOrigin()) { // Call local isInsecureOrigin
    statusPopoverInstance.displayInsecureOriginWarning('geolocation');
  }
  statusPopoverInstance.showGeolocationError(message);
  $container.find('#pos_status').html('Geolocation unavailable.');
}

function updatePositionGeolocation(pos, $container) {
  let d;
  if (/^((?!chrome|android).)*safari/i.test(navigator.userAgent)) {
    d = new Date();
  } else {
    d = new Date(pos.timestamp);
  }
  $container.find('#pos_status').html('Last position update ' + dateFormat(d) + '...');
  $container.find('#pos_lat').val(pos.coords.latitude);
  $container.find('#pos_long').val(pos.coords.longitude);
  $container.find('#pos_ts').val(d.getTime());
}

function updatePositionGPS(gpsData, $container) {
  const d = new Date(gpsData.Time);
  $container.find('#pos_status').html('Last position update ' + dateFormat(d) + '...');
  $container.find('#pos_lat').val(gpsData.Lat);
  $container.find('#pos_long').val(gpsData.Lon);
  $container.find('#pos_ts').val(d.getTime());
}

function postPosition($container) {
  const pos = {
    lat: parseFloat($container.find('#pos_lat').val()),
    lon: parseFloat($container.find('#pos_long').val()),
    comment: $container.find('#pos_comment').val(),
    date: new Date(parseInt($container.find('#pos_ts').val())),
  };

  $.ajax('/api/posreport', {
    data: JSON.stringify(pos),
    contentType: 'application/json',
    type: 'POST',
    success: function(resp) {
      $container.modal('toggle');
      alert(resp);
    },
    error: function(xhr, st, resp) {
      alert(resp + ': ' + xhr.responseText);
    },
  });
}

export function initGeolocation(options) {
  const $container = $(options.containerSelector);
  if (!$container.length) {
    console.error('Geolocation module: Container not found', options.containerSelector);
    return;
  }

  $container.find('#pos_btn').click(function() {
    postPosition($container);
  });

  $container.on('shown.bs.modal', function(e) {
    $.ajax({
      url: '/api/current_gps_position',
      dataType: 'json',
      beforeSend: function() {
        $container.find('#pos_status').html('Checking if GPS device is available');
      },
      success: function(gpsData) {
        $container.find('#pos_status').html('GPS position received');
        $container.find('#pos_status').html('<strong>Waiting for position from GPS device...</strong>');
        updatePositionGPS(gpsData, $container);
      },
      error: function(jqXHR, textStatus, errorThrown) {
        $container.find('#pos_status').html('GPS device not available!');
        if (navigator.geolocation) {
          $container.find('#pos_status').html('<strong>Waiting for position (geolocation)...</strong>');
          const geoOptions = { enableHighAccuracy: true, maximumAge: 0 };
          posId = navigator.geolocation.watchPosition(
            (pos) => updatePositionGeolocation(pos, $container),
            (error) => handleGeolocationError(error, $container, options.statusPopoverInstance),
            geoOptions
          );
        } else {
          $container.find('#pos_status').html('Geolocation is not supported by this browser.');
        }
      },
    });
  });

  $container.on('hidden.bs.modal', function(e) {
    if (navigator.geolocation && posId !== 0) {
      navigator.geolocation.clearWatch(posId);
      posId = 0;
    }
  });
}
