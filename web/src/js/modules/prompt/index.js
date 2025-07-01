export class PromptModal {
  static getInstance(modalSelector = '#promptModal') {
    if (!PromptModal.instance) {
      PromptModal.instance = new PromptModal(modalSelector);
    }
    return PromptModal.instance;
  }
  constructor(modalSelector = '#promptModal') {
    this.modal = $(modalSelector);
    this.modalBody = this.modal.find('.modal-body');
    this.modalFooter = this.modal.find('.modal-footer');
    this.modalMessage = $('#promptMessage');
    this.currentNotification = null;
    this.onResponse = null;

    // Ensure prompt modal appears on top
    this.modal.css('z-index', 1050);
  }

  _cleanupModals() {
    // Close any open modals first
    $('.modal').modal('hide');
    // Remove any stuck backdrops
    $('.modal-backdrop').remove();
    $('body').removeClass('modal-open');
  }

  showCustom(options = {}) {
    this._cleanupModals();

    // Clear previous content
    this.modalBody.empty();
    this.modalFooter.empty();

    // Set message if provided
    if (options.message) {
      this.modalMessage.text(options.message);
    }

    // Handle custom content
    if (options.body) {
      this.modalBody.append(options.body);
    }

    // Add buttons
    if (options.buttons) {
      options.buttons.forEach(button => {
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: `btn ${button.class || 'btn-default'}`
            })
            .addClass(button.pullLeft ? 'pull-left' : '')
            .text(button.text)
            .click(e => {
              if (button.onClick) {
                button.onClick(e);
              }
              if (button.close !== false) {
                this.hide();
              }
            })
        );
      });
    }

    // Show the modal
    this.modal.modal({
      backdrop: options.static === true ? 'static' : true,
      keyboard: options.keyboard !== false,
      show: true
    });
  }

  showSystemPrompt(prompt, onResponse) {
    this._cleanupModals();

    // Clear previous content
    this.modalBody.empty();
    this.modalFooter.empty();

    this.onResponse = onResponse;
    this._handleSystemPrompt(prompt);
  }

  hide() {
    this.modal.modal('hide');
    if (this.currentNotification) {
      this.currentNotification.close();
      this.currentNotification = null;
    }
  }

  setNotification(notification) {
    if (this.currentNotification) {
      this.currentNotification.close();
    }
    this.currentNotification = notification;
  }


  // Handles system prompts (password, busy-channel, multi-select)
  _handleSystemPrompt(prompt) {
    // Store prompt ID
    this.modalBody.append($('<input type="hidden">').attr({
      id: 'promptID',
      value: prompt.id
    }));

    // Set prompt message and kind
    this.modalMessage.text(prompt.message);
    this.modal.data('prompt-kind', prompt.kind);

    switch (prompt.kind) {
      case 'password':
        this.modalBody.append(
          $('<input>').attr({
            type: 'password',
            id: 'promptPasswordInput',
            class: 'form-control',
            placeholder: 'Enter password...',
            autocomplete: 'off'
          })
        );
        this.modalFooter.append(
          $('<input>').attr({
            type: 'submit',
            class: 'btn btn-primary',
            id: 'promptOkButton',
            value: 'OK'
          }).click(() => {
            this._submitResponse($('#promptPasswordInput').val());
          })
        );
        break;

      case 'busy-channel':
        this.modalBody.append(
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
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: 'btn btn-default',
              id: 'promptOkButton'
            })
            .text('Continue anyway')
            .click(() => {
              this._submitResponse('continue');
            })
        );
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: 'btn btn-primary'
            })
            .text('Abort')
            .click(() => {
              this._submitResponse('abort');
            })
        );
        break;

      case 'multi-select':
        const container = $('<div>').addClass('checkbox-list');
        const list = $('<ul>').addClass('checkbox-list-items');

        prompt.options.forEach(opt => {
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
        this.modalBody.append(container);

        // Add select all toggle button
        this.modalFooter.append(
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

        this.modalFooter.append(
          $('<input>')
            .attr({
              type: 'submit',
              class: 'btn btn-primary',
              id: 'promptOkButton',
              value: 'OK'
            })
            .click(() => {
              const value = $('.modal-body .checkbox-list input:checked')
                .map(function() { return $(this).val(); })
                .get()
                .join(',');
              this._submitResponse(value);
            })
        );
        break;

      case 'pre-account-activation':
        this.modalBody.append(
          $('<p>').addClass('text-warning').html('<b>WARNING:</b> We were unable to confirm that your Winlink account is active.')
        ).append(
          $('<p>').text('If you continue, an over-the-air activation will be initiated and you will receive a message with a new password.')
        ).append(
          $('<p>').text('This password will be the only key to your account. If you lose it, it cannot be recovered.')
        ).append(
          $('<p>').html('It is strongly recommended to create your account before proceeding.')
        );
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: 'btn btn-default pull-left',
            })
            .text('Continue anyway')
            .click(() => {
              this._submitResponse('confirmed');
            })
        );
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: 'btn btn-default',
            })
            .text('Abort')
            .click(() => {
              this._submitResponse('abort');
            })
        );
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: 'btn btn-primary',
            })
            .text('Create new account')
            .click(() => {
              this._submitResponse('abort');
              window.location.href = '/ui/config?action=create-account';
            })
        );
        break;

      case 'account-activation':
        this.modalBody.append(
          $('<p>').text('Welcome! The system has automatically generated a password for your new account.')
        ).append(
          $('<p>').text('This password is in a message that is ready to be downloaded to your inbox during this session.')
        ).append(
          $('<p>').addClass('text-warning').html('<b>WARNING:</b> Once you download this message, the password inside is the only key to your account. If you lose it, it cannot be recovered.')
        ).append(
          $('<p>').text('Are you ready to receive this message and save the password securely right now?')
        );
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: 'btn btn-default',
            })
            .text('Postpone to Next Connection')
            .click(() => {
              this._submitResponse('defer');
            })
        );
        this.modalFooter.append(
          $('<button>')
            .attr({
              type: 'button',
              class: 'btn btn-primary',
            })
            .text('Yes, Download Now')
            .click(() => {
              this._submitResponse('accept');
            })
        );
        break;

      default:
        console.log('Ignoring unsupported prompt kind:', prompt.kind);
        return;
    }

    // Show modal
    this.modal.modal({
      backdrop: 'static',
      keyboard: false,
      show: true
    });
  }

  _submitResponse(value) {
    const id = $('#promptID').val();
    this.hide();
    if (this.onResponse) {
      this.onResponse({
        id: id,
        value: value
      });
    }
  }
}
PromptModal.instance = null;
