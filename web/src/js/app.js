import 'jquery';
import 'bootstrap/dist/js/bootstrap';
import 'bootstrap-select';
import 'bootstrap-tokenfield';

import { Version } from './modules/version/index.js';
import { NotificationService } from './modules/notifications/index.js';
import { StatusPopover } from './modules/status-popover/index.js';
import { Geolocation } from './modules/geolocation/index.js';
import { alert, htmlEscape, isImageSuffix, formatFileSize, formXmlToFormName } from './modules/utils/index.js';
import { ConnectModal } from './modules/connect-modal/index.js';
import { PromptModal } from './modules/prompt/index.js';
import { PasswordRecovery } from './modules/password-recovery/main.js';
import { Mailbox } from './modules/mailbox/index.js';
import { Composer } from './modules/composer/index.js';
import { FormCatalog } from './modules/form-catalog/index.js';
import { Viewer } from './modules/viewer/index.js';

let wsURL = '';
let mycall = '';

let ws;
let configHash; // For auto-reload on config changes

let statusPopover;
let promptModal;
let connectModal;
let version;
let notificationService;
let passwordRecovery;
let geolocation;
let mailbox;
let composer;
let formCatalog;
let viewer;

$(document).ready(function() {
  wsURL = (location.protocol == 'https:' ? 'wss://' : 'ws://') + location.host + '/ws';

  $(function() {
    mycall = $('#mycall').text();

    statusPopover = new StatusPopover();
    statusPopover.init();
    connectModal = new ConnectModal(mycall);
    connectModal.init();
    promptModal = new PromptModal();
    promptModal.init();
    version = new Version(promptModal);
    passwordRecovery = new PasswordRecovery(promptModal, statusPopover, mycall);
    passwordRecovery.init();
    geolocation = new Geolocation(statusPopover);
    geolocation.init();
    notificationService = new NotificationService(statusPopover);
    notificationService.init();
    composer = new Composer(mycall);
    composer.init();
    viewer = new Viewer(composer);
    viewer.init();
    mailbox = new Mailbox((currentFolder, mid) => viewer.displayMessage(currentFolder, mid));
    mailbox.init();
    formCatalog = new FormCatalog(composer);
    formCatalog.init();


    $('.nav :not(.dropdown) a').on('click', function() {
      if ($('.navbar-toggle').css('display') != 'none') {
        $('.navbar-toggle').trigger('click');
      }
    });

    $('#updateFormsButton').click(formCatalog.update);

    initWs();
    mailbox.displayFolder('in');

    version.checkNewVersion();
  });
});

let cancelCloseTimer = false;

function updateProgress(p) {
  cancelCloseTimer = !p.done;

  if (p.receiving || p.sending) {
    const percent = Math.ceil((p.bytes_transferred * 100) / p.bytes_total);
    const op = p.receiving ? 'Receiving' : 'Sending';
    let text = op + ' ' + p.mid + ' (' + p.bytes_total + ' bytes)';
    if (p.subject) {
      text += ' - ' + htmlEscape(p.subject);
    }
    $('#navbar_progress .progress-text').text(text);
    $('#navbar_progress .progress-bar')
      .css('width', percent + '%')
      .text(percent + '%');
  }

  if ($('#navbar_progress').is(':visible') && p.done) {
    window.setTimeout(function() {
      if (!cancelCloseTimer) {
        $('#navbar_progress').fadeOut(500);
      }
    }, 3000);
  } else if ((p.receiving || p.sending) && !p.done) {
    $('#navbar_progress').show();
  }
}

function updateStatus(data) {
  const st = $('#status_text');
  st.empty().off('click').attr('data-toggle', 'tooltip').attr('data-placement', 'bottom').tooltip();

  const onDisconnect = function() {
    st.tooltip('hide');
    disconnect(false, () => {
      // This will be reset by the next updateStatus when the session is aborted
      st.empty().append('Disconnecting... ');
      // Issue dirty disconnect on second click
      st.off('click').click(() => {
        st.off('click');
        disconnect(true);
        st.tooltip('hide');
      });
      st.attr('title', 'Click to force disconnect').tooltip('fixTitle').tooltip('show');
    });
  };

  if (data.dialing) {
    st.append('Dialing... ');
    st.click(onDisconnect);
    st.attr('title', 'Click to abort').tooltip('fixTitle').tooltip('show');
  } else if (data.connected) {
    st.append('Connected ' + data.remote_addr);
    st.click(onDisconnect);
    st.attr('title', 'Click to disconnect').tooltip('fixTitle').tooltip('hide');
  } else {
    if (data.active_listeners.length > 0) {
      st.append('<i>Listening ' + data.active_listeners + '</i>');
    } else {
      st.append('<i>Ready</i>');
    }
    st.attr('title', 'Click to connect').tooltip('fixTitle').tooltip('hide');
    st.click(() => { connectModal.toggle(); });
  }

  const n = data.http_clients.length;
  statusPopover
    .showWebserverInfo(n + (n == 1 ? ' client ' : ' clients ') + 'connected.');
}

function disconnect(dirty, successHandler) {
  if (successHandler === undefined) {
    successHandler = () => { };
  }
  $.post(
    '/api/disconnect?dirty=' + dirty,
    {},
    function(response) {
      successHandler();
    },
    'json'
  );
}

function initWs() {
  if ('WebSocket' in window) {
    ws = new WebSocket(wsURL);

    ws.onopen = function(evt) {
      console.log('Websocket opened');
      statusPopover.hideWebsocketError();
      statusPopover.showWebserverInfo(); // Content is updated by updateStatus
      $('#console').empty();
      setTimeout(() => {
        passwordRecovery.checkPasswordRecoveryEmail();
      }, 3000);
    };
    ws.onmessage = function(evt) {
      const msg = JSON.parse(evt.data);
      if (msg.MyCall) {
        mycall = msg.MyCall;
      }
      if (msg.Notification) {
        notificationService.show(msg.Notification.title, msg.Notification.body);
      }
      if (msg.LogLine) {
        updateConsole(msg.LogLine + '\n');
      }
      if (msg.UpdateMailbox) {
        mailbox.displayFolder(mailbox.currentFolder);
      }
      if (msg.Status) {
        if (configHash && configHash !== msg.Status.config_hash) {
          if ($('#composer').is(':visible')) {
            const div = $('#navbar_status');
            div.empty();
            div.append(
              '<span class="navbar-text status-text text-warning"><span class="glyphicon glyphicon-warning-sign"></span> Configuration has changed, please <a href="#" onclick="location.reload()">reload the page</a>.</span>'
            );
            div.show();
          } else {
            console.log('Config hash changed, reloading page');
            location.reload();
          }
        }
        configHash = msg.Status.config_hash;
        updateStatus(msg.Status);
      }
      if (msg.Progress) {
        updateProgress(msg.Progress);
      }
      if (msg.Prompt) {
        promptModal.showSystemPrompt(msg.Prompt, (response) => {
          ws.send(JSON.stringify({ prompt_response: response }));
        });
        promptModal.setNotification(notificationService.show(msg.Prompt.message, ''));
      }
      if (msg.PromptAbort) {
        promptModal.hide();
      }
      if (msg.Ping) {
        ws.send(JSON.stringify({ Pong: true }));
      }
    };
    ws.onclose = function(evt) {
      console.log('Websocket closed');
      statusPopover.showWebsocketError("WebSocket connection closed. Attempting to reconnect...");
      statusPopover.hideWebserverInfo();
      $('#status_text').empty();
      window.setTimeout(function() {
        initWs();
      }, 1000);
    };
  } else {
    // The browser doesn't support WebSocket
    alert('Websocket not supported by your browser, please upgrade your browser.');
  }
}

function updateConsole(msg) {
  const pre = $('#console');
  pre.append('<span class="terminal">' + msg + '</span>');
  pre.scrollTop(pre.prop('scrollHeight'));
}
