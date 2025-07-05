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
    mailbox = new Mailbox(displayMessage);
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
    let wsError = true;
    alert('Websocket not supported by your browser, please upgrade your browser.');
  }
}

function updateConsole(msg) {
  const pre = $('#console');
  pre.append('<span class="terminal">' + msg + '</span>');
  pre.scrollTop(pre.prop('scrollHeight'));
}

function displayMessage(elem, currentFolder) {
  const mid = elem.attr('ID');
  const msg_url = buildMessagePath(currentFolder, mid);

  $.getJSON(msg_url, function(data) {
    elem.attr('class', 'info');

    const view = $('#message_view');
    view.find('#subject').text(data.Subject);
    view.find('#headers').empty();
    view.find('#headers').append('Date: ' + data.Date + '<br>');
    view.find('#headers').append('From: ' + data.From.Addr + '<br>');
    view.find('#headers').append('To: ');
    for (let i = 0; data.To && i < data.To.length; i++) {
      view
        .find('#headers')
        .append('<el>' + data.To[i].Addr + '</el>' + (data.To.length - 1 > i ? ', ' : ''));
    }
    if (data.P2POnly) {
      view.find('#headers').append(' (<strong>P2P only</strong>)');
    }

    if (data.Cc) {
      view.find('#headers').append('<br>Cc: ');
      for (let i = 0; i < data.Cc.length; i++) {
        view
          .find('#headers')
          .append('<el>' + data.Cc[i].Addr + '</el>' + (data.Cc.length - 1 > i ? ', ' : ''));
      }
    }

    view.find('#body').html(data.BodyHTML);

    const attachments = view.find('#attachments');
    attachments.empty();

    // Add a row container
    const row = $('<div class="row"></div>');
    attachments.append(row);

    if (!data.Files) {
      attachments.hide();
    } else {
      attachments.show();
    }
    for (let i = 0; data.Files && i < data.Files.length; i++) {
      const file = data.Files[i];
      const formName = formXmlToFormName(file.Name);
      let renderToHtml = 'false';
      if (formName) {
        renderToHtml = 'true';
      }
      const attachUrl = msg_url + '/' + file.Name + '?rendertohtml=' + renderToHtml;

      const col = $('<div class="col-xs-6 col-md-3"></div>');
      const link = $('<a class="attachment-preview"></a>');

      if (isImageSuffix(file.Name)) {
        link.attr('target', '_blank').attr('href', msg_url + '/' + file.Name);
        link.html(
          '<span class="filesize">' + formatFileSize(file.Size) + '</span>' +
          '<span class="glyphicon glyphicon-paperclip"></span> ' +
          '<img src="' + msg_url + '/' + file.Name + '" alt="' + file.Name + '">'
        );
        col.append(link);
        attachments.append(col);
      } else if (formName) {
        attachments.append(
          '<div class="col-xs-6 col-md-3"><a target="_blank" href="' +
          attachUrl +
          '" class="btn btn-default navbar-btn"><span class="glyphicon glyphicon-edit"></span> ' +
          formName +
          '</a></div>'
        );
      } else {
        link.attr('target', '_blank').attr('href', msg_url + '/' + file.Name);
        link.html(
          '<span class="filesize">' + formatFileSize(file.Size) + '</span>' +
          '<span class="glyphicon glyphicon-paperclip"></span> ' +
          '<br><span class="filename">' + file.Name + '</span>'
        );
        col.append(link);
        attachments.append(col);
      }
    }
    $('#reply_btn').off('click');
    $('#reply_btn').click(function(evt) {
      composer.reply(currentFolder, data, false);
    });

    $('#reply_all_btn').click(function(evt) {
      composer.reply(currentFolder, data, true);
    });
    $('#forward_btn').off('click');
    $('#forward_btn').click(function(evt) {
      composer.forward(currentFolder, data);
    });
    $('#edit_as_new_btn').off('click');
    $('#edit_as_new_btn').click(function(evt) {
      composer.editAsNew(currentFolder, data);
    });
    $('#delete_btn').off('click');
    $('#delete_btn').click(function(evt) {
      deleteMessage(currentFolder, mid);
    });
    $('#archive_btn').off('click');
    $('#archive_btn').click(function(evt) {
      archiveMessage(currentFolder, mid);
    });

    // Archive button should be hidden for already archived messages
    if (currentFolder == 'archive') {
      $('#archive_btn').parent().hide();
    } else {
      $('#archive_btn').parent().show();
    }

    view.show();
    $('#message_view').modal('show');
    let mbox = currentFolder;
    if (!data.Read) {
      window.setTimeout(function() {
        setRead(mbox, data.MID);
      }, 2000);
    }
    elem.attr('class', 'active');
  });
}

function archiveMessage(box, mid) {
  $.ajax('/api/mailbox/archive', {
    headers: {
      'X-Pat-SourcePath': buildMessagePath(box, mid),
    },
    contentType: 'application/json',
    type: 'POST',
    success: function(resp) {
      $('#message_view').modal('hide');
      alert('Message archived');
    },
    error: function(xhr, st, resp) {
      alert(resp + ': ' + xhr.responseText);
    },
  });
}

function deleteMessage(box, mid) {
  $('#confirm_delete').on('click', '.btn-ok', function(e) {
    $('#message_view').modal('hide');
    const $modalDiv = $(e.delegateTarget);
    $.ajax(buildMessagePath(box, mid), {
      type: 'DELETE',
      success: function(resp) {
        $modalDiv.modal('hide');
        alert('Message deleted');
      },
      error: function(xhr, st, resp) {
        $modalDiv.modal('hide');
        alert(resp + ': ' + xhr.responseText);
      },
    });
  });
  $('#confirm_delete').modal('show');
}

function setRead(box, mid) {
  const data = { read: true };

  $.ajax(buildMessagePath(box, mid) + '/read', {
    data: JSON.stringify(data),
    contentType: 'application/json',
    type: 'POST',
    success: function(resp) { },
    error: function(xhr, st, resp) {
      alert(resp + ': ' + xhr.responseText);
    },
  });
}

function buildMessagePath(folder, mid) {
  return '/api/mailbox/' + encodeURIComponent(folder) + '/' + encodeURIComponent(mid);
}
