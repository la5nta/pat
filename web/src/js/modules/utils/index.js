import $ from 'jquery';

export function alert(msg) {
  const div = $('#navbar_status');
  div.empty();
  div.append('<span class="navbar-text status-text">' + msg + '</span>');
  div.show();
  window.setTimeout(function() {
    div.fadeOut(500);
  }, 5000);
}

export function isInsecureOrigin() {
  if (Object.prototype.hasOwnProperty.call(window, 'isSecureContext')) {
    return !window.isSecureContext;
  }
  if (window.location.protocol === 'https:') {
    return false;
  }
  if (window.location.protocol === 'file:') {
    return false;
  }
  if (window.location.hostname === 'localhost' || window.location.hostname.startsWith('127.')) {
    return false;
  }
  return true;
}

export function dateFormat(previous) {
  const current = new Date();
  const msPerMinute = 60 * 1000;
  const msPerHour = msPerMinute * 60;
  const msPerDay = msPerHour * 24;
  const msPerMonth = msPerDay * 30;
  const msPerYear = msPerDay * 365;
  const elapsed = current - previous;

  if (elapsed < msPerDay) {
    return (
      (previous.getHours() < 10 ? '0' : '') +
      previous.getHours() +
      ':' +
      (previous.getMinutes() < 10 ? '0' : '') +
      previous.getMinutes()
    );
  } else if (elapsed < msPerMonth) {
    return 'approximately ' + Math.round(elapsed / msPerDay) + ' days ago';
  } else if (elapsed < msPerYear) {
    return 'approximately ' + Math.round(elapsed / msPerMonth) + ' months ago';
  } else {
    return 'approximately ' + Math.round(elapsed / msPerYear) + ' years ago';
  }
}

export function htmlEscape(str) {
  return $('<div></div>').text(str).html();
}

export function isImageSuffix(name) {
  return name.toLowerCase().match(/\.(jpg|jpeg|png|gif)$/);
}

export function formatFileSize(bytes) {
  if (bytes >= 1024 * 1024) {
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  } else if (bytes >= 1024) {
    return (bytes / 1024).toFixed(1) + ' KB';
  }
  return bytes + ' B';
}
