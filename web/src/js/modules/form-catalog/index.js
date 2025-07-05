export class FormCatalog {
  constructor(composer) {
    this.composer = composer;
    this.formsCatalog = null;
  }

  init() {
    $.getJSON('/api/formcatalog')
      .done((data) => {
        this._initFormSelect(data);
        // Add search handlers
        $('#formSearchInput').on('input', (e) => {
          this._filterForms($(e.target).val().toLowerCase());
        });

        $('#clearSearchButton').click(() => {
          $('#formSearchInput').val('');
          this._filterForms('');
        });
      })
      .fail((data) => {
        this._initFormSelect(null);
      });
  }

  update() {
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
            this.init();
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

  _filterForms(searchTerm) {
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

  _initFormSelect(data) {
    this.formsCatalog = data;
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
      this._appendFormFolder('formFolderRoot', data);
    } else {
      $('#formsRootFolderName').text('missing form templates');
      $(`#formFolderRoot`).append(`
        <h6>Form templates not downloaded</h6>
        Use Action → Update Form Templates to download now
      `);
    }
  }

  _appendFormFolder(rootId, data, level = 0) {
    if (!data.folders && !data.forms) return;

    const container = $(`#${rootId}`);

    // Handle folders
    if (data.folders && data.folders.length > 0) {
      data.folders.forEach((folder) => {
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
          this._appendFormFolder(`${folderContentId} .folder-content`, folder, level + 1);
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
          this._onFormLaunching(`/api/forms?template=${path}${replyParam}`);
        });

        formsContainer.append(formDiv);
      });
      container.append(formsContainer);
    }
  }

  _onFormLaunching(target) {
    $('#selectForm').modal('hide');
    this.composer.startPolling();
    window.open(target);
  }
}
