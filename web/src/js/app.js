import 'jquery';
import 'bootstrap/dist/js/bootstrap';
import 'bootstrap-select';
import 'bootstrap-tokenfield';

import { alert } from './modules/utils/index.js';
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
import { StatusText } from './modules/status-text/index.js';

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
let statusText;

$(function() {
  wsURL = (location.protocol == 'https:' ? 'wss://' : 'ws://') + location.host + '/ws';
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
  statusText = new StatusText(() => connectModal.toggle());
  statusText.init();

  // Setup folder navigation
  $('a[data-folder]').on('click', (e) => {
    const folder = $(e.currentTarget).data('folder');
    mailbox.displayFolder(folder);
  });

  // Highlight active tab (mailbox folder)
  $('a[data-folder]').parent('li').on('click', (e) => {
    $('.navbar li.active').removeClass('active');
    const $this = $(e.currentTarget);
    if (!$this.hasClass('active')) {
      $this.addClass('active');
    }
    e.preventDefault();
  });
  $('.nav :not(.dropdown) a').on('click', () => {
    if ($('.navbar-toggle').css('display') != 'none') {
      $('.navbar-toggle').trigger('click');
    }
  });

  initWs();
  mailbox.displayFolder('in');
  version.checkNewVersion();
});

function initWs() {
  if (!('WebSocket' in window)) {
    // The browser doesn't support WebSocket
    alert('Websocket not supported by your browser, please upgrade your browser.');
    return;
  }

  ws = new WebSocket(wsURL);
  ws.onopen = () => {
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
      statusText.update(msg.Status);
      const n = msg.Status.http_clients.length;
      statusPopover
        .showWebserverInfo(n + (n == 1 ? ' client ' : ' clients ') + 'connected.');
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
  ws.onclose = () => {
    console.log('Websocket closed');
    statusPopover.showWebsocketError("WebSocket connection closed. Attempting to reconnect...");
    statusPopover.hideWebserverInfo();
    $('#status_text').empty();
    window.setTimeout(function() {
      initWs();
    }, 1000);
  };
}

function updateConsole(msg) {
  const pre = $('#console');
  pre.append('<span class="terminal">' + msg + '</span>');
  pre.scrollTop(pre.prop('scrollHeight'));
}
