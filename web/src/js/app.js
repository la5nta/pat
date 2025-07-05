import 'jquery';
import 'bootstrap/dist/js/bootstrap';
import 'bootstrap-select';
import 'bootstrap-tokenfield';

import { alert, htmlEscape } from './modules/utils/index.js';
import { Version } from './modules/version/index.js';
import { NotificationService } from './modules/notifications/index.js';
import { StatusPopover } from './modules/status-popover/index.js';
import { Geolocation } from './modules/geolocation/index.js';
import { ConnectModal } from './modules/connect-modal/index.js';
import { PromptModal } from './modules/prompt/index.js';
import { PasswordRecovery } from './modules/password-recovery/main.js';
import { Mailbox } from './modules/mailbox/index.js';
import { Composer } from './modules/composer/index.js';
import { FormCatalog } from './modules/form-catalog/index.js';
import { Viewer } from './modules/viewer/index.js';
import { ProgressBar } from './modules/progress-bar/index.js';

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
let progressBar;

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
    progressBar = new ProgressBar();
    progressBar.init();

    // Setup folder navigation
    $('#inbox_tab').click(() => mailbox.displayFolder('in'));
    $('#outbox_tab').click(() => mailbox.displayFolder('out'));
    $('#sent_tab').click(() => mailbox.displayFolder('sent'));
    $('#archive_tab').click(() => mailbox.displayFolder('archive'));

    // Highlight active tab (mailbox folder)
    $('#inbox_tab, #outbox_tab, #sent_tab, #archive_tab')
      .parent('li')
      .click(function(e) {
        $('.navbar li.active').removeClass('active');
        const $this = $(this);
        if (!$this.hasClass('active')) {
          $this.addClass('active');
        }
        e.preventDefault();
      });
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
        progressBar.update(msg.Progress);
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
