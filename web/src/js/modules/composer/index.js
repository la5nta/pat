import { alert, isImageSuffix, formatFileSize, formXmlToFormName, setCookie, deleteCookie } from '../utils/index.js';

const tokenfieldConfig = {
  delimiter: [',', ';', ' '], // Must be in sync with SplitFunc (utils.go)
  inputType: 'email',
  createTokensOnBlur: true,
};

export class Composer {
  constructor(mycall) {
    this.mycall = mycall;
    this.pollTimer = null;
  }

  startPolling() {
    setCookie('forminstance', Math.floor(Math.random() * 1000000000), 1);
    this._poll();
  }

  _forgetPolling() {
    window.clearTimeout(this.pollTimer);
    deleteCookie('forminstance');
  }

  _poll() {
    $.ajax({
      method: 'GET',
      url: '/api/form',
      dataType: 'json',
      success: (data) => {
        // TODO: Should verify forminstance key in case of multi-user scenario
        console.log('done polling');
        console.log(data);
        if (!$('#composer').hasClass('hidden')) {
          this._writeToComposer(data);
        }
      },
      error: () => {
        if (!$('#composer').hasClass('hidden')) {
          // TODO: Consider replacing this polling mechanism with a WS message (push)
          this.pollTimer = window.setTimeout(this._poll.bind(this), 1000);
        }
      },
    });
  }

  _writeToComposer(data) {
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



  init() {
    $('#compose_btn').click((evt) => {
      this.close(true); // Clear everything when opening a new compose
      $('#composer').modal('toggle');
    });

    $('#msg_to').tokenfield(tokenfieldConfig);
    $('#msg_cc').tokenfield(tokenfieldConfig);
    $('#composer').on('change', '.btn-file :file', this._handleFileSelection.bind(this));
    $('#composer').on('hidden.bs.modal', this._forgetPolling.bind(this));

    $('#composer_error').hide();

    $('#compose_cancel').click((evt) => {
      this.close(true);
    });

    $('#composer_form').submit((e) => {
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
        success: (result) => {
          // Clear stored files data
          $('#msg_attachments_input')[0].dataset.storedFiles = '[]';
          $('#composer').modal('hide');
          this.close(true);
          alert(result);
        },
        error: (error) => {
          $('#composer_error').html(error.responseText);
          $('#composer_error').show();
        },
      });
      e.preventDefault();
    });
  }

  close(clear) {
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

  _handleFileSelection() {
    const fileInput = $('#msg_attachments_input')[0];
    const dt = new DataTransfer();
    let storedFiles = [];
    let filesProcessed = 0;
    const totalFiles = fileInput.files.length;

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
    Array.from(fileInput.files).forEach(file => {
      const reader = new FileReader();
      reader.onload = (e) => {
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
          this._previewAttachmentFiles.call(fileInput);
        }
      };
      reader.readAsDataURL(file);
    });
  }

  _previewAttachmentFiles() {
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

  _reAttachFiles(msg_url, files) {
    $('#composer_attachments').empty();
    const fileInput = $('#msg_attachments_input')[0];
    const dt = new DataTransfer();

    if (files) {
      let filesProcessed = 0;
      files.forEach(file => {
        $.ajax({
          url: msg_url + '/' + file.Name,
          method: 'GET',
          xhrFields: {
            responseType: 'blob'
          },
          success: (blob) => {
            const f = new File([blob], file.Name, { type: blob.type });
            dt.items.add(f);
            filesProcessed++;

            if (filesProcessed === files.length) {
              fileInput.files = dt.files;
              this._previewAttachmentFiles.call(fileInput);
            }
          }
        });
      });
    }
  }

  reply(folder, data, replyAll) {
    $('#message_view').modal('hide');

    $('#msg_to').tokenfield('setTokens', [data.From.Addr]);
    $('#msg_cc').tokenfield('setTokens', replyAll ? this._replyCarbonCopyList(data) : []);
    if (data.Subject.lastIndexOf('Re:', 0) != 0) {
      $('#msg_subject').val('Re: ' + data.Subject);
    } else {
      $('#msg_subject').val(data.Subject);
    }
    $('#msg_body').val('\n\n' + this._quoteMsg(data));
    $('#composer').data('in-reply-to', folder + '/' + data.MID);
    $('#composer').modal('show');
    $('#msg_body').focus();
    $('#msg_body')[0].setSelectionRange(0, 0);

    // opens browser window for a form-based reply,
    // or does nothing if this is not a form-based message
    this._showReplyForm(folder, data.MID, data);
  }

  forward(folder, data) {
    $('#message_view').modal('hide');

    $('#msg_to').tokenfield('setTokens', '');
    $('#msg_subject').val('Fw: ' + data.Subject);
    $('#msg_body').val(this._quoteMsg(data));
    $('#msg_body')[0].setSelectionRange(0, 0);

    // Add attachments
    this._reAttachFiles(this._buildMessagePath(folder, data.MID), data.Files);

    $('#composer').modal('show');
    $('#msg_to-tokenfield').focus();
  }

  editAsNew(folder, data) {
    $('#message_view').modal('hide');

    $('#msg_to').tokenfield('setTokens', data.To.map(function(recipient) { return recipient.Addr; }));
    $('#msg_cc').tokenfield('setTokens', data.Cc ? data.Cc.map(function(recipient) { return recipient.Addr; }) : []);
    $('#msg_subject').val(data.Subject);
    $('#msg_body').val(data.Body);
    $('#msg_body')[0].setSelectionRange(0, 0);

    // Add attachments
    this._reAttachFiles(this._buildMessagePath(folder, data.MID), data.Files);

    $('#composer').modal('show');
    $('#msg_to-tokenfield').focus();
  }

  _quoteMsg(data) {
    let output = '--- ' + data.Date + ' ' + data.From.Addr + ' wrote: ---\n';

    const lines = data.Body.split('\n');
    for (let i = 0; i < lines.length; i++) {
      output += '>' + lines[i] + '\n';
    }
    return output;
  }

  _replyCarbonCopyList(msg) {
    let addrs = msg.To;
    if (msg.Cc != null && msg.Cc.length > 0) {
      addrs = addrs.concat(msg.Cc);
    }
    const seen = {};
    seen[this.mycall] = true;
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

  _showReplyForm(folder, mid, msg) {
    const orgMsgUrl = this._buildMessagePath(folder, mid);
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
        (data) => {
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
              window.setTimeout(() => this.startPolling(), 500);
              open(attachUrl + '?rendertohtml=true&in-reply-to=' + encodeURIComponent(folder + '/' + mid));
            }
          }
        },
        'text'
      );
      return;
    }
  }

  _buildMessagePath(folder, mid) {
    return '/api/mailbox/' + encodeURIComponent(folder) + '/' + encodeURIComponent(mid);
  }
}
