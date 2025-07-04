import { isInsecureOrigin } from '../utils/index.js';

export class NotificationService {
  constructor(statusPopover) {
    this.statusPopover = statusPopover;
  }
  init() {
    this.requestSystemPermission();
  }

  isSupported() {
    if (!window.Notification || !Notification.requestPermission) return false;
    if (Notification.permission === 'granted') return true;

    // Chrome on Android support notifications only in the context of a Service worker.
    // This is a hack to detect this case, so we can avoid asking for a pointless permission.
    try {
      new Notification('');
    } catch (e) {
      if (e.name == 'TypeError') return false;
    }
    return true;
  }

  requestSystemPermission() {
    if (!this.isSupported()) {
      const notificationsErrorPanelBody = this.statusPopover.getNotificationsErrorPanelBody();
      notificationsErrorPanelBody.html('Not supported by this browser.');
      this.statusPopover.showNotificationsErrorPanel();
      return;
    }

    Notification.requestPermission((permission) => {
      const notificationsErrorPanelBody = this.statusPopover.getNotificationsErrorPanelBody();

      if (permission === 'granted') {
        this.statusPopover.hideNotificationsErrorPanel();
      } else if (isInsecureOrigin()) {
        // There is no way of knowing for sure if the permission was denied by the user
        // or prohibited because of insecure origin (Chrome). This is just a lucky guess.
        this.statusPopover.displayInsecureOriginWarning('notifications');
      } else {
        // Permission denied or dismissed by user
        notificationsErrorPanelBody.html('Notification permission denied or dismissed.');
        this.statusPopover.showNotificationsErrorPanel();
      }
    });
  }

  show(title, body = '') {
    if (this.isSupported() && Notification.permission === 'granted') {
      const options = { body, icon: '/dist/static/pat_logo.png' };
      return new Notification(title, options);
    }
    return null;
  }
}
