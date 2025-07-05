import { htmlEscape } from '../utils/index.js';

export class ProgressBar {
  constructor() {
    this.cancelCloseTimer = false;
  }

  init() {
    this.progressBar = $('#navbar_progress');
  }

  update(p) {
    this.cancelCloseTimer = !p.done;

    if (p.receiving || p.sending) {
      const percent = Math.ceil((p.bytes_transferred * 100) / p.bytes_total);
      const op = p.receiving ? 'Receiving' : 'Sending';
      let text = op + ' ' + p.mid + ' (' + p.bytes_total + ' bytes)';
      if (p.subject) {
        text += ' - ' + htmlEscape(p.subject);
      }
      this.progressBar.find('.progress-text').text(text);
      this.progressBar
        .find('.progress-bar')
        .css('width', percent + '%')
        .text(percent + '%');
    }

    if (this.progressBar.is(':visible') && p.done) {
      window.setTimeout(() => {
        if (!this.cancelCloseTimer) {
          this.progressBar.fadeOut(500);
        }
      }, 3000);
    } else if ((p.receiving || p.sending) && !p.done) {
      this.progressBar.show();
    }
  }
}
