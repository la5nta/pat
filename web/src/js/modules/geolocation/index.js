import { alert, isInsecureOrigin, dateFormat } from '../utils/index.js';

export class Geolocation {
  constructor(statusPopover) {
    this.statusPopover = statusPopover;
    this.$container = $('#posModal');
    this.posId = 0;
  }

  init() {
    if (!this.$container.length) {
      console.error('Geolocation module: Container not found');
      return;
    }

    this.$container.find('#pos_btn').click(() => {
      this.postPosition();
    });

    this.$container.on('shown.bs.modal', () => {
      this.onModalShown();
    });

    this.$container.on('hidden.bs.modal', () => {
      this.onModalHidden();
    });
  }

  handleGeolocationError(error) {
    let message = 'Geolocation error.';
    if (error && error.message) {
      message = error.message;
    }
    if (error.message.search('insecure origin') > 0 || isInsecureOrigin()) {
      this.statusPopover.displayInsecureOriginWarning('geolocation');
    }
    this.statusPopover.showGeolocationError(message);
    this.$container.find('#pos_status').html('Geolocation unavailable.');
  }

  updatePositionGeolocation(pos) {
    let d;
    if (/^((?!chrome|android).)*safari/i.test(navigator.userAgent)) {
      d = new Date();
    } else {
      d = new Date(pos.timestamp);
    }
    this.$container.find('#pos_status').html('Last position update ' + dateFormat(d) + '...');
    this.$container.find('#pos_lat').val(pos.coords.latitude);
    this.$container.find('#pos_long').val(pos.coords.longitude);
    this.$container.find('#pos_ts').val(d.getTime());
  }

  updatePositionGPS(gpsData) {
    const d = new Date(gpsData.Time);
    this.$container.find('#pos_status').html('Last position update ' + dateFormat(d) + '...');
    this.$container.find('#pos_lat').val(gpsData.Lat);
    this.$container.find('#pos_long').val(gpsData.Lon);
    this.$container.find('#pos_ts').val(d.getTime());
  }

  postPosition() {
    const pos = {
      lat: parseFloat(this.$container.find('#pos_lat').val()),
      lon: parseFloat(this.$container.find('#pos_long').val()),
      comment: this.$container.find('#pos_comment').val(),
      date: new Date(parseInt(this.$container.find('#pos_ts').val())),
    };

    $.ajax('/api/posreport', {
      data: JSON.stringify(pos),
      contentType: 'application/json',
      type: 'POST',
      success: (resp) => {
        this.$container.modal('toggle');
        alert(resp);
      },
      error: (xhr, st, resp) => {
        alert(resp + ': ' + xhr.responseText);
      },
    });
  }

  onModalShown() {
    $.ajax({
      url: '/api/current_gps_position',
      dataType: 'json',
      beforeSend: () => {
        this.$container.find('#pos_status').html('Checking if GPS device is available');
      },
      success: (gpsData) => {
        this.$container.find('#pos_status').html('GPS position received');
        this.$container.find('#pos_status').html('<strong>Waiting for position from GPS device...</strong>');
        this.updatePositionGPS(gpsData);
      },
      error: () => {
        this.$container.find('#pos_status').html('GPS device not available!');
        if (navigator.geolocation) {
          this.$container.find('#pos_status').html('<strong>Waiting for position (geolocation)...</strong>');
          const geoOptions = { enableHighAccuracy: true, maximumAge: 0 };
          this.posId = navigator.geolocation.watchPosition(
            (pos) => this.updatePositionGeolocation(pos),
            (error) => this.handleGeolocationError(error),
            geoOptions
          );
        } else {
          this.$container.find('#pos_status').html('Geolocation is not supported by this browser.');
        }
      },
    });
  }

  onModalHidden() {
    if (navigator.geolocation && this.posId !== 0) {
      navigator.geolocation.clearWatch(this.posId);
      this.posId = 0;
    }
  }
}
