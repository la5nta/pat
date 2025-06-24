import 'jquery';
import 'bootstrap/dist/js/bootstrap';
import 'bootstrap-select';
import 'bootstrap-tokenfield';

import { checkNewVersion } from './modules/version/index.js';
import { NotificationService } from './modules/notifications/index.js';
import { StatusPopover } from './modules/status-popover/index.js';
import { initGeolocation } from './modules/geolocation/index.js';
import { alert } from './modules/utils/index.js';
import { initConnectModal, connect } from './modules/connect-modal/index.js';

let wsURL = '';
let mycall = '';

let formsCatalog;
let currentPromptNotification = null;
let ws;
let configHash; // For auto-reload on config changes

let statusPopover;

$(document).ready(function() {
  wsURL = (location.protocol == 'https:' ? 'wss://' : 'ws://') + location.host + '/ws';

  // Ensure prompt modal appears on top
  $('#promptModal').css('z-index', 1050);

  $(function() {
    initConfigDefaults();
    statusPopover = new StatusPopover('#status_popover_content', '#gui_status_light', '.navbar-brand');

    // Setup actions
    $('#connect_btn').click(connect);
    $('#connectForm input').keypress(function(e) {
      if (e.which == 13) {
        connect();
        return false;
      }
    });
    $('#connectForm input').keyup(function(e) {
      onConnectInputChange();
    });

    // Setup composer
    initComposeModal();

    // Setup folder navigation
    $('#inbox_tab').click(function(evt) {
      displayFolder('in');
    });
    $('#outbox_tab').click(function(evt) {
      displayFolder('out');
    });
    $('#sent_tab').click(function(evt) {
      displayFolder('sent');
    });
    $('#archive_tab').click(function(evt) {
      displayFolder('archive');
    });
    $('#inbox_tab, #outbox_tab, #sent_tab, #archive_tab').parent('li').click(function(e) {
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

    $('#updateFormsButton').click(updateForms);

    initConnectModal();

    initConsole();
    displayFolder('in');

    initNotifications();
    initForms();
    checkNewVersion();

    initGeolocation({
      containerSelector: '#posModal',
      statusPopoverInstance: statusPopover,
    });
  });
});

function initNotifications() {
  NotificationService.requestSystemPermission(statusPopover);
}

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


function onFormLaunching(target) {
  $('#selectForm').modal('hide');
  startPollingFormData();
  window.open(target);
}

function startPollingFormData() {
  setCookie('forminstance', Math.floor(Math.random() * 1000000000), 1);
  pollFormData();
}

function forgetFormData() {
  window.clearTimeout(pollTimer);
  deleteCookie('forminstance');
}

let pollTimer;

function pollFormData() {
  $.ajax({
    method: 'GET',
    url: '/api/form',
    dataType: 'json',
    success: function(data) {
      // TODO: Should verify forminstance key in case of multi-user scenario
      console.log('done polling');
      console.log(data);
      if (!$('#composer').hasClass('hidden')) {
        writeFormDataToComposer(data);
      }
    },
    error: function() {
      if (!$('#composer').hasClass('hidden')) {
        // TODO: Consider replacing this polling mechanism with a WS message (push)
        pollTimer = window.setTimeout(pollFormData, 1000);
      }
    },
  });
}

function writeFormDataToComposer(data) {
  $('#msg_body').val(data.msg_body);
  if (data.msg_to) {
    $('#msg_to').tokenfield('setTokens', data.msg_to.split(/[ ;,]/).filter(Boolean));
  }
  if (data.msg_cc) {
    $('#msg_cc').tokenfield('setTokens', data.msg_cc.split(/[ ;,]/).filter(Boolean));
  }
  if (data.msg_subject) {
    // in case of composing a form-based reply we keep the 'Re: ...' subject line
    $('#msg_subject').val(data.msg_subject);
  }
}

function initComposeModal() {
  $('#compose_btn').click(function(evt) {
    closeComposer(true); // Clear everything when opening a new compose
    $('#composer').modal('toggle');
  });
  const tokenfieldConfig = {
    delimiter: [',', ';', ' '], // Must be in sync with SplitFunc (utils.go)
    inputType: 'email',
    createTokensOnBlur: true,
  };
  $('#msg_to').tokenfield(tokenfieldConfig);
  $('#msg_cc').tokenfield(tokenfieldConfig);
  $('#composer').on('change', '.btn-file :file', handleFileSelection);
  $('#composer').on('hidden.bs.modal', forgetFormData);

  $('#composer_error').hide();

  $('#compose_cancel').click(function(evt) {
    closeComposer(true);
  });

  $('#composer_form').submit(function(e) {
    const form = $('#composer_form');
    const formData = new FormData(form[0]);

    const d = new Date().toJSON();
    formData.append('date', d);

    // Add in-reply-to header if present
    const inReplyTo = $('#composer').data('in-reply-to');
    if (inReplyTo) {
      formData.append('in_reply_to', inReplyTo);
    }

    // Set some defaults that makes the message pass validation
    if ($('#msg_body').val().length == 0) {
      $('#msg_body').val('<No message body>');
    }
    if ($('#msg_subject').val().length == 0) {
      $('#msg_subject').val('<No subject>');
    }

    $.ajax({
      url: '/api/mailbox/out',
      method: 'POST',
      data: formData,
      processData: false,
      contentType: false,
      success: function(result) {
        // Clear stored files data
        $('#msg_attachments_input')[0].dataset.storedFiles = '[]';
        $('#composer').modal('hide');
        closeComposer(true);
        alert(result);
      },
      error: function(error) {
        $('#composer_error').html(error.responseText);
        $('#composer_error').show();
      },
    });
    e.preventDefault();
  });
}

function initForms() {
  $.getJSON('/api/formcatalog')
    .done(function(data) {
      initFormSelect(data);
      // Add search handlers
      $('#formSearchInput').on('input', function() {
        filterForms($(this).val().toLowerCase());
      });

      $('#clearSearchButton').click(function() {
        $('#formSearchInput').val('');
        filterForms('');
      });
    })
    .fail(function(data) {
      initFormSelect(null);
    });
}

function filterForms(searchTerm) {
  let visibleCount = 0;

  // Search through all form items
  $('.form-item').each(function() {
    const formDiv = $(this);
    const templatePath = formDiv.data('template-path') || '';
    const isMatch = templatePath.toLowerCase().includes(searchTerm);

    // Show/hide the form item
    formDiv.css('display', isMatch ? '' : 'none');
    if (isMatch) visibleCount++;
  });

  // Show/hide folders based on whether they have visible forms
  $('.folder-container').each(function() {
    const folder = $(this);
    const hasVisibleForms = folder.find('.form-item').filter(function() {
      return $(this).css('display') !== 'none';
    }).length > 0;
    folder.css('display', hasVisibleForms ? '' : 'none');
  });

  // Auto-expand/collapse based on result count
  if (visibleCount < 20) {
    // Expand when few results
    $('.folder-toggle.collapsed').each(function() {
      $(this).click();
    });
  } else {
    // Collapse when many results
    $('.folder-toggle:not(.collapsed)').each(function() {
      $(this).click();
    });
  }
}

function initFormSelect(data) {
  formsCatalog = data;
  if (
    data &&
    data.path &&
    ((data.folders && data.folders.length > 0) || (data.forms && data.forms.length > 0))
  ) {
    $('#formsVersion').html(
      '<span>(ver <a href="http://www.winlink.org/content/all_standard_templates_folders_one_zip_self_extracting_winlink_express_ver_12142016">' +
      data.version +
      '</a>)</span>'
    );
    $('#updateFormsVersion').html(data.version);
    $('#formsRootFolderName').text(data.path);
    $('#formFolderRoot').html('');
    appendFormFolder('formFolderRoot', data);
  } else {
    $('#formsRootFolderName').text('missing form templates');
    $(`#formFolderRoot`).append(`
			<h6>Form templates not downloaded</h6>
			Use Action → Update Form Templates to download now
			`);
  }
}

function updateForms() {
  $('#updateFormsResponse').text('');
  $('#updateFormsError').text('');

  // Disable button and show spinner
  const btn = $('#updateFormsButton');
  const spinner = $('#updateFormsSpinner');
  btn.prop('disabled', true);
  spinner.show().addClass('icon-spin');

  $.ajax({
    method: 'POST',
    url: '/api/formsUpdate',
    success: (msg) => {
      $('#updateFormsError').text('');
      let response = JSON.parse(msg);
      switch (response.action) {
        case 'none':
          $('#updateFormsResponse').text('You already have the latest forms version');
          break;
        case 'update':
          $('#updateFormsResponse').text('Updated forms to ' + response.newestVersion);
          // Update views to reflect new state
          initForms();
          break;
      }
    },
    error: (err) => {
      $('#updateFormsResponse').text('');
      $('#updateFormsError').text(err.responseText);
    },
    complete: () => {
      // Re-enable button and hide spinner
      btn.prop('disabled', false);
      spinner.hide().removeClass('icon-spin');
    }
  });
}

function setCookie(cname, cvalue, exdays) {
  const d = new Date();
  d.setTime(d.getTime() + exdays * 24 * 60 * 60 * 1000);
  const expires = 'expires=' + d.toUTCString();
  document.cookie = cname + '=' + cvalue + ';' + expires + ';path=/';
}

function deleteCookie(cname) {
  document.cookie = cname + '=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;';
}

function appendFormFolder(rootId, data, level = 0) {
  if (!data.folders && !data.forms) return;

  const container = $(`#${rootId}`);

  // Handle folders
  if (data.folders && data.folders.length > 0) {
    data.folders.forEach(function(folder) {
      if (folder.form_count > 0) {
        // Create unique IDs for this folder
        const folderContentId = `folder-content-${Math.random().toString(36).substr(2, 9)}`;

        // Create the folder structure
        const folderDiv = $(`
          <div class="folder-container ${level > 0 ? 'nested-folder' : ''}">
            <button class="btn btn-secondary folder-toggle mb-2 collapsed"
                    data-toggle="collapse"
                    data-target="#${folderContentId}">
              ${folder.name}
            </button>
            <div id="${folderContentId}" class="collapse">
              <div class="folder-content"></div>
            </div>
          </div>
        `);

        container.append(folderDiv);

        // Recursively add sub-folders and forms
        appendFormFolder(`${folderContentId} .folder-content`, folder, level + 1);
      }
    });
  }

  // Handle forms at this level
  if (data.forms && data.forms.length > 0) {
    const formsContainer = $('<div class="forms-container"></div>');
    data.forms.forEach((form) => {
      const formDiv = $(`
        <div class="form-item">
          <button class="btn btn-light btn-block" style="text-align: left">
            ${form.name}
          </button>
        </div>
      `).data('template-path', form.template_path);

      formDiv.find('button').on('click', () => {
        const inReplyTo = $('#composer').data('in-reply-to');
        const replyParam = inReplyTo ? '&in-reply-to=' + encodeURIComponent(inReplyTo) : '';
        const path = encodeURIComponent(form.template_path);
        onFormLaunching(`/api/forms?template=${path}${replyParam}`);
      });

      formsContainer.append(formDiv);
    });
    container.append(formsContainer);
  }
}



















// Handle file selection and deduplication
function handleFileSelection() {
  const fileInput = this;
  const dt = new DataTransfer();
  let storedFiles = [];
  let filesProcessed = 0;
  const totalFiles = this.files.length;

  // Get previously stored files from data attribute
  try {
    storedFiles = JSON.parse(fileInput.dataset.storedFiles || '[]');

    // First add all previously stored files to DataTransfer
    storedFiles.forEach(fileInfo => {
      const byteString = atob(fileInfo.content.split(',')[1]);
      const ab = new ArrayBuffer(byteString.length);
      const ia = new Uint8Array(ab);
      for (let i = 0; i < byteString.length; i++) {
        ia[i] = byteString.charCodeAt(i);
      }
      const blob = new Blob([ab], { type: fileInfo.type });
      const file = new File([blob], fileInfo.name, { type: fileInfo.type });
      dt.items.add(file);
    });
  } catch (e) {
    console.error("Error parsing stored files:", e);
  }

  // Process newly selected files
  Array.from(this.files).forEach(file => {
    const reader = new FileReader();
    reader.onload = function(e) {
      // Add to stored files array
      storedFiles.push({
        name: file.name,
        type: file.type,
        content: e.target.result
      });

      // Update dataset
      fileInput.dataset.storedFiles = JSON.stringify(storedFiles);

      // Add to DataTransfer
      dt.items.add(file);

      filesProcessed++;

      // Only update input files and preview when ALL files are processed
      if (filesProcessed === totalFiles) {
        fileInput.files = dt.files;
        previewAttachmentFiles.call(fileInput);
      }
    };
    reader.readAsDataURL(file);
  });
}

// Display file previews
function previewAttachmentFiles() {
  const attachments = $('#composer_attachments');
  attachments.empty();

  // Add a row container
  const row = $('<div class="row"></div>');
  attachments.append(row);

  for (let i = 0; i < this.files.length; i++) {
    const file = this.files[i];

    const col = $('<div class="col-xs-6 col-md-3"></div>');
    const link = $('<a class="attachment-preview"></a>');

    // Add remove button - append it directly to avoid event binding issues
    const removeBtn = $('<button type="button" class="close remove-attachment" aria-label="Remove">' +
      '<span aria-hidden="true">&times;</span></button>');
    removeBtn.click((e) => {
      e.preventDefault();
      e.stopPropagation(); // Prevent event from bubbling up
      // Remove file from DataTransfer
      const dt = new DataTransfer();
      const files = this.files;
      for (let i = 0; i < files.length; i++) {
        if (files[i].name !== file.name) {
          dt.items.add(files[i]);
        }
      }
      this.files = dt.files;
      // Remove preview
      col.remove();
    });

    if (isImageSuffix(file.name)) {
      const reader = new FileReader();
      reader.onload = function(e) {
        link.empty() // Clear any existing content
          .append(removeBtn)
          .append($('<span class="filesize">').text(formatFileSize(file.size)))
          .append($('<img>').attr({
            src: e.target.result,
            alt: file.name
          }));
      };
      reader.readAsDataURL(file);
    } else {
      link.empty() // Clear any existing content
        .append(removeBtn)
        .append($('<span class="filesize">').text(formatFileSize(file.size)))
        .append('<br>')
        .append($('<span class="filename">').text(file.name));
    }

    col.append(link);
    row.append(col);
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
    st.click(() => {
      $('#connectModal').modal('toggle');
    });
  }

  const n = data.http_clients.length;
  statusPopover
    .showWebserverInfo(n + (n == 1 ? ' client ' : ' clients ') + 'connected.');
}

function closeComposer(clear) {
  if (clear) {
    $('#composer_error').val('').hide();
    $('#msg_body').val('');
    $('#msg_subject').val('');
    $('#msg_to').tokenfield('setTokens', []);
    $('#msg_cc').tokenfield('setTokens', []);
    $('#composer_form')[0].reset();
    $('#composer').removeData('in-reply-to');

    // Attachment previews
    $('#composer_attachments').empty();

    // Attachment input field
    let attachments = $('#msg_attachments_input');
    attachments[0].dataset.storedFiles = '[]';
    attachments.replaceWith((attachments = attachments.clone(true)));
  }
  $('#composer').modal('hide');
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

function initConfigDefaults() {
  $.getJSON('/api/config')
    .done(function(config) {
      if (config.ardop && config.ardop.connect_requests) {
        $('#connectRequestsInput').attr('placeholder', config.ardop.connect_requests);
      }
    })
    .fail(function() {
      console.log("Failed to load config defaults");
    });
}

function initConsole() {
  if ('WebSocket' in window) {
    ws = new WebSocket(wsURL);
    ws.onopen = function(evt) {
      console.log('Websocket opened');
      statusPopover.hideWebsocketError();
      statusPopover.showWebserverInfo(); // Content is updated by updateStatus
      $('#console').empty();
    };
    ws.onmessage = function(evt) {
      const msg = JSON.parse(evt.data);
      if (msg.MyCall) {
        mycall = msg.MyCall;
      }
      if (msg.Notification) {
        NotificationService.show(msg.Notification.title, msg.Notification.body);
      }
      if (msg.LogLine) {
        updateConsole(msg.LogLine + '\n');
      }
      if (msg.UpdateMailbox) {
        displayFolder(currentFolder);
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
        processPromptQuery(msg.Prompt);
        if (currentPromptNotification) {
          currentPromptNotification.close();
        }
        currentPromptNotification = NotificationService.show(msg.Prompt.message, '');
      }
      if (msg.PromptAbort) {
        $('#promptModal').modal('hide');
        if (currentPromptNotification) {
          currentPromptNotification.close();
          currentPromptNotification = null;
        }
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
        initConsole();
      }, 1000);
    };
  } else {
    // The browser doesn't support WebSocket
    let wsError = true;
    alert('Websocket not supported by your browser, please upgrade your browser.');
  }
}

function processPromptQuery(p) {
  console.log(p);

  // Close any open modals first
  $('.modal').modal('hide');
  // Remove any stuck backdrops
  $('.modal-backdrop').remove();
  $('body').removeClass('modal-open');

  const modal = $('#promptModal');
  const modalBody = modal.find('.modal-body');
  const modalFooter = modal.find('.modal-footer');

  // Clear previous content
  modalBody.empty();
  modalFooter.empty();

  // Add hidden prompt ID
  modalBody.append($('<input type="hidden">').attr({
    id: 'promptID',
    value: p.id
  }));

  // Set prompt message and kind
  $('#promptMessage').text(p.message);
  modal.data('prompt-kind', p.kind);

  // Show relevant input based on type
  switch (p.kind) {
    case 'password':
      modalBody.append(
        $('<input>')
          .attr({
            type: 'password',
            id: 'promptPasswordInput',
            class: 'form-control',
            placeholder: 'Enter password...',
            autocomplete: 'off'
          })
      );
      modalFooter.append(
        $('<input>')
          .attr({
            type: 'submit',
            class: 'btn btn-primary',
            id: 'promptOkButton',
            value: 'OK'
          })
          .click(function() {
            submitPromptResponse($('#promptPasswordInput').val());
          })
      );
      break;

    case 'busy-channel':
      modalBody.append(
        $('<div>')
          .addClass('text-center')
          .append($('<span>')
            .addClass('glyphicon glyphicon-refresh icon-spin text-muted')
            .css({
              'font-size': '36px',
              'margin': '12px 0'
            })
          )
      );
      modalFooter.append(
        $('<button>')
          .attr({
            type: 'button',
            class: 'btn btn-default',
            id: 'promptOkButton'
          })
          .text('Continue anyway')
          .click(function() {
            const id = $('#promptID').val();
            $('#promptModal').modal('hide');
            submitPromptResponse('continue');
          })
      );
      modalFooter.append(
        $('<button>')
          .attr({
            type: 'button',
            class: 'btn btn-primary'
          })
          .text('Abort')
          .click(function() {
            const id = $('#promptID').val();
            $('#promptModal').modal('hide');
            submitPromptResponse('abort');
          })
      );
      break;

    case 'multi-select':
      const container = $('<div>').addClass('checkbox-list');
      const list = $('<ul>').addClass('checkbox-list-items');

      p.options.forEach(opt => {
        const li = $('<li>');
        const label = $('<label>').addClass('checkbox-item');
        const input = $('<input>').attr({
          type: 'checkbox',
          value: opt.value,
          checked: opt.checked
        });
        label.append(input);
        label.append(` ${opt.desc || opt.value} (${opt.value})`);
        li.append(label);
        list.append(li);
      });

      container.append(list);
      modalBody.append(container);

      // Add select all toggle button
      modalFooter.append(
        $('<button>')
          .attr({
            type: 'button',
            class: 'btn btn-default pull-left',
            id: 'selectAllToggle'
          })
          .text('Select All')
          .click(function() {
            const checkboxes = container.find('input[type="checkbox"]');
            const allSelected = checkboxes.filter(':checked').length === checkboxes.length;
            checkboxes.prop('checked', !allSelected);
            $(this).text(!allSelected ? 'Deselect All' : 'Select All');
            $(this).blur();
          })
      );

      modalFooter.append(
        $('<input>')
          .attr({
            type: 'submit',
            class: 'btn btn-primary',
            id: 'promptOkButton',
            value: 'OK'
          })
          .click(function() {
            const value = $('.modal-body .checkbox-list input:checked')
              .map(function() { return $(this).val(); })
              .get()
              .join(',');
            submitPromptResponse(value);
          })
      );
      break;

    default:
      console.log('Ignoring unsupported prompt kind:', p.kind);
      return;
  }


  // Show modal with error handling
  try {
    $('#promptModal').modal({
      backdrop: 'static', // Prevent closing by clicking outside
      keyboard: false,    // Prevent closing with keyboard
      show: true
    });
  } catch (e) {
    console.error('Failed to show prompt modal:', e);
    // Attempt recovery
    $('.modal-backdrop').remove();
    $('body').removeClass('modal-open');
    $('#promptModal').modal('hide');
    setTimeout(() => {
      $('#promptModal').modal('show');
    }, 100);
  }
}

function submitPromptResponse(value) {
  const id = $('#promptID').val();
  $('#promptModal').modal('hide');
  ws.send(JSON.stringify({
    prompt_response: {
      id: id,
      value: value
    }
  }));
}

function updateConsole(msg) {
  const pre = $('#console');
  pre.append('<span class="terminal">' + msg + '</span>');
  pre.scrollTop(pre.prop('scrollHeight'));
}

const getCellValue = (tr, idx) => tr.children[idx].innerText || tr.children[idx].textContent;

const comparer = (idx, asc) => (a, b) =>
  ((v1, v2) =>
    v1 !== '' && v2 !== '' && !isNaN(v1) && !isNaN(v2) ? v1 - v2 : v1.toString().localeCompare(v2))(
      getCellValue(asc ? a : b, idx),
      getCellValue(asc ? b : a, idx)
    );

let currentFolder;

function displayFolder(dir) {
  currentFolder = dir;

  const is_from = dir == 'in' || dir == 'archive';

  const table = $('#folder table');
  table.empty();
  table.append(
    '<thead><tr><th></th><th>Subject</th>' +
    '<th>' +
    (is_from ? 'From' : 'To') +
    '</th>' +
    (is_from ? '' : '<th>P2P</th>') +
    '<th>Date</th><th>Message ID</th></tr></thead><tbody></tbody>'
  );

  const tbody = $('#folder table tbody');

  $.getJSON('/api/mailbox/' + dir, function(data) {
    for (let i = 0; i < data.length; i++) {
      const msg = data[i];

      //TODO: Cleanup (Sorry about this...)
      let html =
        '<tr id="' + msg.MID + '" class="active' + (msg.Unread ? ' strong' : '') + '"><td>';
      if (msg.Files.length > 0) {
        html += '<span class="glyphicon glyphicon-paperclip"></span>';
      }
      html += '</td><td>' + htmlEscape(msg.Subject) + '</td><td>';
      if (!is_from && !msg.To) {
        html += '';
      } else if (is_from) {
        html += msg.From.Addr;
      } else if (msg.To.length == 1) {
        html += msg.To[0].Addr;
      } else if (msg.To.length > 1) {
        html += msg.To[0].Addr + '...';
      }
      html += '</td>';
      html += is_from
        ? ''
        : '<td>' + (msg.P2POnly ? '<span class="glyphicon glyphicon-ok"></span>' : '') + '</td>';
      html += '<td>' + msg.Date + '</td><td>' + msg.MID + '</td></tr>';

      const elem = $(html);
      tbody.append(elem);
      elem.click(function(evt) {
        displayMessage($(this));
      });
    }
  });
  // Adapted from https://stackoverflow.com/a/49041392
  document.querySelectorAll('th').forEach((th) =>
    th.addEventListener('click', () => {
      const table = th.closest('table');
      const tbody = table.querySelector('tbody');
      Array.from(tbody.querySelectorAll('tr'))
        .sort(comparer(Array.from(th.parentNode.children).indexOf(th), (this.asc = !this.asc)))
        .forEach((tr) => tbody.appendChild(tr));
      const previousTh = table.querySelector('th.sorted');
      if (previousTh != null) {
        previousTh.classList.remove('sorted');
      }
      th.classList.add('sorted');
    })
  );
}

function displayMessage(elem) {
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
      handleReply(false);
    });

    $('#reply_all_btn').click(function(evt) {
      handleReply(true);
    });
    $('#forward_btn').off('click');
    $('#forward_btn').click(function(evt) {
      $('#message_view').modal('hide');

      $('#msg_to').tokenfield('setTokens', '');
      $('#msg_subject').val('Fw: ' + data.Subject);
      $('#msg_body').val(quoteMsg(data));
      $('#msg_body')[0].setSelectionRange(0, 0);

      // Add attachments
      $('#composer_attachments').empty();
      const fileInput = $('#msg_attachments_input')[0];
      const dt = new DataTransfer();

      if (data.Files) {
        let filesProcessed = 0;
        data.Files.forEach(file => {
          $.ajax({
            url: msg_url + '/' + file.Name,
            method: 'GET',
            xhrFields: {
              responseType: 'blob'
            },
            success: function(blob) {
              const f = new File([blob], file.Name, { type: blob.type });
              dt.items.add(f);
              filesProcessed++;

              if (filesProcessed === data.Files.length) {
                fileInput.files = dt.files;
                previewAttachmentFiles.call(fileInput);
              }
            }
          });
        });
      }

      $('#composer').modal('show');
      $('#msg_to-tokenfield').focus();
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

    // Add reply handling function
    function handleReply(replyAll) {
      $('#message_view').modal('hide');

      $('#msg_to').tokenfield('setTokens', [data.From.Addr]);
      $('#msg_cc').tokenfield('setTokens', replyAll ? replyCarbonCopyList(data) : []);
      if (data.Subject.lastIndexOf('Re:', 0) != 0) {
        $('#msg_subject').val('Re: ' + data.Subject);
      } else {
        $('#msg_subject').val(data.Subject);
      }
      $('#msg_body').val('\n\n' + quoteMsg(data));
      $('#composer').data('in-reply-to', currentFolder + '/' + mid);
      $('#composer').modal('show');
      $('#msg_body').focus();
      $('#msg_body')[0].setSelectionRange(0, 0);

      // opens browser window for a form-based reply,
      // or does nothing if this is not a form-based message
      showReplyForm(currentFolder, mid, data);
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

function formXmlToFormName(fileName) {
  let match = fileName.match(/^RMS_Express_Form_([\w \.]+)-\d+\.xml$/i);
  if (match) {
    return match[1];
  }

  match = fileName.match(/^RMS_Express_Form_([\w \.]+)\.xml$/i);
  if (match) {
    return match[1];
  }

  return null;
}

function showReplyForm(mbox, mid, msg) {
  const orgMsgUrl = buildMessagePath(mbox, mid);
  for (let i = 0; msg.Files && i < msg.Files.length; i++) {
    const file = msg.Files[i];
    const formName = formXmlToFormName(file.Name);
    if (!formName) {
      continue;
    }
    // retrieve form XML attachment and determine if it specifies a form-based reply
    const attachUrl = orgMsgUrl + '/' + file.Name;
    $.get(
      attachUrl + '?rendertohtml=false',
      {},
      function(data) {
        let parser = new DOMParser();
        let xmlDoc = parser.parseFromString(data, 'text/xml');
        if (xmlDoc) {
          let replyTmpl = xmlDoc.evaluate(
            '/RMS_Express_Form/form_parameters/reply_template',
            xmlDoc,
            null,
            XPathResult.STRING_TYPE,
            null
          );
          if (replyTmpl && replyTmpl.stringValue) {
            window.setTimeout(startPollingFormData, 500);
            open(attachUrl + '?rendertohtml=true&in-reply-to=' + encodeURIComponent(mbox + "/" + mid));
          }
        }
      },
      'text'
    );
    return;
  }
}

function replyCarbonCopyList(msg) {
  let addrs = msg.To;
  if (msg.Cc != null && msg.Cc.length > 0) {
    addrs = addrs.concat(msg.Cc);
  }
  const seen = {};
  seen[mycall] = true;
  seen[msg.From.Addr] = true;
  const strings = [];
  for (let i = 0; i < addrs.length; i++) {
    if (seen[addrs[i].Addr]) {
      continue;
    }
    seen[addrs[i].Addr] = true;
    strings.push(addrs[i].Addr);
  }
  return strings;
}

function quoteMsg(data) {
  let output = '--- ' + data.Date + ' ' + data.From.Addr + ' wrote: ---\n';

  const lines = data.Body.split('\n');
  for (let i = 0; i < lines.length; i++) {
    output += '>' + lines[i] + '\n';
  }
  return output;
}

function htmlEscape(str) {
  return $('<div></div>').text(str).html();
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

function isImageSuffix(name) {
  return name.toLowerCase().match(/\.(jpg|jpeg|png|gif)$/);
}

function formatFileSize(bytes) {
  if (bytes >= 1024 * 1024) {
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  } else if (bytes >= 1024) {
    return (bytes / 1024).toFixed(1) + ' KB';
  }
  return bytes + ' B';
}

function buildMessagePath(folder, mid) {
  return '/api/mailbox/' + encodeURIComponent(folder) + '/' + encodeURIComponent(mid);
}
