import $ from 'jquery';

export class StatusPopover {
  constructor() {
    this.statusPopoverDiv = null;
    this.guiStatusLight = null;
    this.navbarBrand = null;
    this._panelSelectors = {
      websocketError: '#websocket_error',
      webserverInfo: '#webserver_info',
      notificationsError: '#notifications_error',
      geolocationError: '#geolocation_error',
      noError: '#no_error',
    };
  }

  init() {
    this.statusPopoverDiv = $('#status_popover_content');
    this.guiStatusLight = $('#gui_status_light');
    this.navbarBrand = $('.navbar-brand');
    this._initPopover();
  }

  _initPopover() {
    this.showWebsocketError("Attempting to connect to WebSocket..."); // Initial message
    this.showNotificationsErrorPanel(); // Content will be managed by NotificationService

    this.guiStatusLight.popover({
      placement: 'bottom',
      content: this.statusPopoverDiv,
      html: true,
    });

    // Hack to force popover to grab it's content div
    this.guiStatusLight.popover('show');
    this.guiStatusLight.popover('hide');
    this.statusPopoverDiv.show();

    // Bind click on navbar-brand
    this.navbarBrand.off('click.statusPopover').on('click.statusPopover', (e) => {
      this.guiStatusLight.popover('toggle');
    });
  }

  addSection({ severity, title, body }) {
    const panelClass = `panel-${severity}`;
    const newSection = $(`
        <div class="panel panel-sm ${panelClass}" data-severity="${severity}">
            <div class="panel-heading"></div>
            <div class="panel-body"></div>
        </div>
    `);

    newSection.find('.panel-heading').text(title);
    newSection.find('.panel-body').append(body);

    const severityOrder = { danger: 3, warning: 2, info: 1 };
    const newSeverity = severityOrder[severity] || 0;

    let inserted = false;
    this.statusPopoverDiv.find('.panel[data-severity]').each(function() {
      const existingSeverity = severityOrder[$(this).data('severity')] || 0;
      if (newSeverity > existingSeverity) {
        $(this).before(newSection);
        inserted = true;
        return false; // break loop
      }
    });

    if (!inserted) {
      this.find(this._panelSelectors.noError).before(newSection);
    }

    this.updateGUIStatus();
    return newSection;
  }

  removeSection(section) {
    if (section && section.length) {
      $(section).remove();
      this.updateGUIStatus();
    }
  }

  _setPanelState(panelSelector, isVisible, content = null, isHtml = false) {
    const panel = this.find(panelSelector);
    if (!panel.length) {
      return;
    }
    if (content !== null) {
      const panelBody = panel.find('.panel-body');
      if (panelBody.length) {
        isHtml ? panelBody.html(content) : panelBody.text(content);
      } else {
        console.warn(`StatusPopover: Panel "${panelSelector}" is missing a .panel-body, cannot set content.`);
      }
    }
    this.showGUIStatus(panel, isVisible);
    this.updateGUIStatus();
  }

  updateGUIStatus() {
    let color = 'success';
    this.statusPopoverDiv
      .find('.panel-info')
      .not('.hidden')
      .not('.ignore-status')
      .each(function(i) {
        color = 'info';
      });
    this.statusPopoverDiv
      .find('.panel-warning')
      .not('.hidden')
      .not('.ignore-status')
      .each(function(i) {
        color = 'warning';
      });
    this.statusPopoverDiv
      .find('.panel-danger')
      .not('.hidden')
      .not('.ignore-status')
      .each(function(i) {
        color = 'danger';
      });
    this.guiStatusLight
      .removeClass(function(index, className) {
        return (className.match(/(^|\s)btn-\S+/g) || []).join(' ');
      })
      .addClass('btn-' + color);

    if (color === 'success') {
      this.showGUIStatus(this.find(this._panelSelectors.noError), true);
    } else {
      this.showGUIStatus(this.find(this._panelSelectors.noError), false);
    }
  }

  showGUIStatus(element, show) {
    const $element = $(element);
    show ? $element.removeClass('hidden') : $element.addClass('hidden');
  }

  find(selector) {
    return this.statusPopoverDiv.find(selector);
  }

  showWebsocketError(message = "WebSocket Connection Error") {
    this._setPanelState(this._panelSelectors.websocketError, true, message);
  }
  hideWebsocketError() {
    this._setPanelState(this._panelSelectors.websocketError, false);
  }

  showWebserverInfo(htmlMessage = "Webserver active") {
    this._setPanelState(this._panelSelectors.webserverInfo, true, htmlMessage, true);
  }
  hideWebserverInfo() {
    this._setPanelState(this._panelSelectors.webserverInfo, false);
  }

  showNotificationsErrorPanel() {
    this._setPanelState(this._panelSelectors.notificationsError, true);
  }
  hideNotificationsErrorPanel() {
    this._setPanelState(this._panelSelectors.notificationsError, false);
  }
  getNotificationsErrorPanelBody() {
    return this.find(this._panelSelectors.notificationsError).find('.panel-body');
  }
  getNotificationsErrorPanel() {
    return this.find(this._panelSelectors.notificationsError);
  }

  showGeolocationError(message = "Geolocation error") {
    this._setPanelState(this._panelSelectors.geolocationError, true, message);
  }
  hideGeolocationError() {
    this._setPanelState(this._panelSelectors.geolocationError, false);
  }
  getGeolocationErrorPanel() {
    return this.find(this._panelSelectors.geolocationError);
  }

  displayInsecureOriginWarning(panelKey) {
    let panelSelector;
    if (panelKey === 'geolocation') {
      panelSelector = this._panelSelectors.geolocationError;
    } else if (panelKey === 'notifications') {
      panelSelector = this._panelSelectors.notificationsError;
    } else {
      console.warn('[displayInsecureOriginWarning] Unknown panelKey:', panelKey);
      return;
    }

    const panel = this.find(panelSelector);
    if (panel.length) {
      panel.removeClass('panel-info').addClass('panel-warning');
      const panelBody = panel.find('.panel-body');
      // Clear previous content before appending to avoid duplicate messages
      panelBody.find('p.insecure-origin-warning').remove();
      panelBody.append('<p class="insecure-origin-warning">Ensure the <a href="https://github.com/la5nta/pat/wiki/The-web-GUI#powerful-features">secure origin criteria for Powerful Features</a> are met.</p>');
      this.showGUIStatus(panel, true);
      this.updateGUIStatus();
    }
  }

  show() {
    this.guiStatusLight.popover('show');
  }
}
