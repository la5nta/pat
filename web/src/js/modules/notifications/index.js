import { isInsecureOrigin } from '../utils/index.js';

export class NotificationService {
  static isSupported() {
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

  static requestSystemPermission(statusPopoverInstance) {
    if (!this.isSupported()) {
      const notificationsErrorPanelBody = statusPopoverInstance.getNotificationsErrorPanelBody();
      notificationsErrorPanelBody.html('Not supported by this browser.');
      statusPopoverInstance.showNotificationsErrorPanel();
      return;
    }

    Notification.requestPermission(function(permission) {
      const notificationsErrorPanelBody = statusPopoverInstance.getNotificationsErrorPanelBody();

      if (permission === 'granted') {
        statusPopoverInstance.hideNotificationsErrorPanel();
      } else if (isInsecureOrigin()) {
        // There is no way of knowing for sure if the permission was denied by the user
        // or prohibited because of insecure origin (Chrome). This is just a lucky guess.
        statusPopoverInstance.displayInsecureOriginWarning('notifications');
      } else {
        // Permission denied or dismissed by user
        notificationsErrorPanelBody.html('Notification permission denied or dismissed.');
        statusPopoverInstance.showNotificationsErrorPanel();
      }
    });
  }

  static show(title, body = '') {
    if (this.isSupported() && Notification.permission === 'granted') {
      const options = { body, icon: '/dist/static/pat_logo.png' };
      return new Notification(title, options);
    }
    return null;
  }
}
