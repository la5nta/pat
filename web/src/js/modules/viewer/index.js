import { alert, isImageSuffix, formatFileSize, formXmlToFormName } from '../utils/index.js';

export class Viewer {
  constructor(composer) {
    this.composer = composer;
  }

  init() {
    this.view = $('#message_view');
    this.subject = this.view.find('#subject');
    this.headers = this.view.find('#headers');
    this.body = this.view.find('#body');
    this.attachments = this.view.find('#attachments');
    this.replyBtn = $('#reply_btn');
    this.replyAllBtn = $('#reply_all_btn');
    this.forwardBtn = $('#forward_btn');
    this.editAsNewBtn = $('#edit_as_new_btn');
    this.deleteBtn = $('#delete_btn');
    this.archiveBtn = $('#archive_btn');
    this.confirmDelete = $('#confirm_delete');
  }

  _buildMessagePath(folder, mid) {
    return '/api/mailbox/' + encodeURIComponent(folder) + '/' + encodeURIComponent(mid);
  }

  _setRead(box, mid) {
    const data = { read: true };

    $.ajax(this._buildMessagePath(box, mid) + '/read', {
      data: JSON.stringify(data),
      contentType: 'application/json',
      type: 'POST',
      success: function(resp) { },
      error: function(xhr, st, resp) {
        alert(resp + ': ' + xhr.responseText);
      },
    });
  }

  _deleteMessage(box, mid) {
    this.confirmDelete.on('click', '.btn-ok', e => {
      this.view.modal('hide');
      const $modalDiv = $(e.delegateTarget);
      $.ajax(this._buildMessagePath(box, mid), {
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
    this.confirmDelete.modal('show');
  }

  _archiveMessage(box, mid) {
    $.ajax('/api/mailbox/archive', {
      headers: {
        'X-Pat-SourcePath': this._buildMessagePath(box, mid),
      },
      contentType: 'application/json',
      type: 'POST',
      success: resp => {
        this.view.modal('hide');
        alert('Message archived');
      },
      error: function(xhr, st, resp) {
        alert(resp + ': ' + xhr.responseText);
      },
    });
  }

  displayMessage(currentFolder, mid) {
    const msg_url = this._buildMessagePath(currentFolder, mid);

    $.getJSON(msg_url, data => {
      this.subject.text(data.Subject);
      this.headers.empty();
      this.headers.append('Date: ' + data.Date + '<br>');
      this.headers.append('From: ' + data.From.Addr + '<br>');
      this.headers.append('To: ');
      for (let i = 0; data.To && i < data.To.length; i++) {
        this.headers.append('<el>' + data.To[i].Addr + '</el>' + (data.To.length - 1 > i ? ', ' : ''));
      }
      if (data.P2POnly) {
        this.headers.append(' (<strong>P2P only</strong>)');
      }

      if (data.Cc) {
        this.headers.append('<br>Cc: ');
        for (let i = 0; i < data.Cc.length; i++) {
          this.headers.append('<el>' + data.Cc[i].Addr + '</el>' + (data.Cc.length - 1 > i ? ', ' : ''));
        }
      }

      this.body.html(data.BodyHTML);

      this.attachments.empty();

      // Add a row container
      const row = $('<div class="row"></div>');
      this.attachments.append(row);

      if (!data.Files) {
        this.attachments.hide();
      } else {
        this.attachments.show();
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
            '<span class="filesize">' +
            formatFileSize(file.Size) +
            '</span>' +
            '<span class="glyphicon glyphicon-paperclip"></span> ' +
            '<img src="' +
            msg_url +
            '/' +
            file.Name +
            '" alt="' +
            file.Name +
            '">'
          );
          col.append(link);
          this.attachments.append(col);
        } else if (formName) {
          this.attachments.append(
            '<div class="col-xs-6 col-md-3"><a target="_blank" href="' +
            attachUrl +
            '" class="btn btn-default navbar-btn"><span class="glyphicon glyphicon-edit"></span> ' +
            formName +
            '</a></div>'
          );
        } else {
          link.attr('target', '_blank').attr('href', msg_url + '/' + file.Name);
          link.html(
            '<span class="filesize">' +
            formatFileSize(file.Size) +
            '</span>' +
            '<span class="glyphicon glyphicon-paperclip"></span> ' +
            '<br><span class="filename">' +
            file.Name +
            '</span>'
          );
          col.append(link);
          this.attachments.append(col);
        }
      }
      this.replyBtn.off('click');
      this.replyBtn.click(evt => {
        this.composer.reply(currentFolder, data, false);
      });

      this.replyAllBtn.click(evt => {
        this.composer.reply(currentFolder, data, true);
      });
      this.forwardBtn.off('click');
      this.forwardBtn.click(evt => {
        this.composer.forward(currentFolder, data);
      });
      this.editAsNewBtn.off('click');
      this.editAsNewBtn.click(evt => {
        this.composer.editAsNew(currentFolder, data);
      });
      this.deleteBtn.off('click');
      this.deleteBtn.click(evt => {
        this._deleteMessage(currentFolder, mid);
      });
      this.archiveBtn.off('click');
      this.archiveBtn.click(evt => {
        this._archiveMessage(currentFolder, mid);
      });

      // Archive button should be hidden for already archived messages
      if (currentFolder == 'archive') {
        this.archiveBtn.parent().hide();
      } else {
        this.archiveBtn.parent().show();
      }

      this.view.show();
      this.view.modal('show');
      let mbox = currentFolder;
      if (!data.Read) {
        window.setTimeout(() => {
          this._setRead(mbox, data.MID);
        }, 2000);
      }
    });
  }
}

