export class PasswordRecovery {
  constructor(promptModal, statusPopover, mycall) {
    this.promptModal = promptModal;
    this.statusPopover = statusPopover;
    this.mycall = mycall;
    this.warningSection = null;
  }

  init() {
    $(document).on('click', '#fix-now-btn', () => this.promptRecoveryEmail());
    $(document).on('click', '#dismiss_password_warning', () => this.dismissPasswordRecoveryWarning());
  }

  checkPasswordRecoveryEmail() {
    // If password recovery email is registered for the account, don't check again for a month
    if (this.wasLastVerified(30 * 24 * 60 * 60 * 1000)) {
      return;
    }

    // Check if user has dismissed the warning within the last 84 hours
    if (this.isPasswordRecoveryDismissed()) {
      return;
    }

    $.ajax('/api/winlink-account/password-recovery-email', {
      type: 'GET',
      timeout: 20000,
      success: (data) => {
        if (!data.recovery_email || data.recovery_email.trim() === '') {
          this.showPasswordRecoveryWarning();
          this.statusPopover.show();
        } else {
          this.setLastVerified();
          this.hidePasswordRecoveryWarning();
        }
      },
      error: (xhr, status, error) => {
        // Log errors silently to console - this request may fail offline
        console.log('Password recovery email check failed (expected when offline):', {
          status: status,
          error: error,
          responseText: xhr.responseText
        });
      }
    });
  }

  showPasswordRecoveryWarning() {
    if (this.warningSection) return;

    const body = $(`
        <div>
            <p>You have no recovery email set for your Winlink account.</p>
            <p>Add one now so you can easily reset a forgotten password.</p>
            <div class="btn-group pull-right" style="margin-top: 10px;">
                <button type="button" class="btn btn-sm btn-default" id="dismiss_password_warning">Later</button>
                <button type="button" class="btn btn-sm btn-primary" id="fix-now-btn">Add Email</button>
            </div>
        </div>
    `);

    this.warningSection = this.statusPopover.addSection({
      severity: 'warning',
      title: 'Secure Your Account',
      body: body,
    });
  }

  hidePasswordRecoveryWarning() {
    if (!this.warningSection) {
      return;
    }
    this.statusPopover.removeSection(this.warningSection);
    this.warningSection = null;
  }

  dismissPasswordRecoveryWarning() {
    const dismissTime = Date.now();
    const dismissUntil = dismissTime + (84 * 60 * 60 * 1000); // 84 hours
    localStorage.setItem(`passwordRecoveryDismissed_${this.mycall}`, dismissUntil.toString());
    this.hidePasswordRecoveryWarning();
  }

  isPasswordRecoveryDismissed() {
    const dismissUntil = localStorage.getItem(`passwordRecoveryDismissed_${this.mycall}`);
    if (!dismissUntil) return false;
    return Date.now() < parseInt(dismissUntil, 10);
  }

  setLastVerified() {
    localStorage.setItem(`passwordRecoveryLastCheck_${this.mycall}`, Date.now().toString());
  }

  wasLastVerified(gracePeriodMillis) {
    const lastCheck = localStorage.getItem(`passwordRecoveryLastCheck_${this.mycall}`);
    if (!lastCheck) return false;
    return (Date.now() - lastCheck) < gracePeriodMillis;
  }

  promptRecoveryEmail() {
    this.promptModal.showCustom({
      message: 'Recovery Email Address',
      body: `
        <p>Please enter your recovery email address. This will be used to recover your password if you forget it.</p>
        <div class="form-group">
          <input type="email" class="form-control" id="recoveryEmail" placeholder="Enter your recovery email">
        </div>
        <div id="recovery-error" class="text-danger" style="display: none;"></div>
          <small class="form-text text-muted">
            By submitting, your email address will be sent directly to winlink.org. Their privacy policy will apply. See <a href="https://winlink.org/terms_conditions" target="_blank">Winlink's Privacy Policy</a>.
          </small>      `,
      buttons: [
        {
          text: 'Cancel',
          class: 'btn-secondary',
          onClick: () => {
            this.promptModal.hide();
          }
        },
        {
          text: 'Submit',
          class: 'btn-primary',
          close: false,
          onClick: (event) => {
            const saveButton = $(event.target);
            const email = $('#recoveryEmail').val();
            const errorContainer = $('#recovery-error');

            // Prepend icon on first click
            if (saveButton.find('span.glyphicon').length === 0) {
              saveButton.prepend('<span class="glyphicon" style="margin-right: 5px; vertical-align: text-bottom;"></span>');
            }
            const icon = saveButton.find('span.glyphicon');

            // The button's text is the last node
            const textNode = saveButton.contents().last()[0];

            saveButton.prop('disabled', true);
            icon.attr('class', 'glyphicon glyphicon-refresh icon-spin');
            textNode.textContent = ' Submitting...';
            errorContainer.hide();

            $.ajax('/api/winlink-account/password-recovery-email', {
              type: 'PUT',
              data: JSON.stringify({ recovery_email: email }),
              contentType: 'application/json',
              timeout: 20000,
              success: () => {
                icon.attr('class', 'glyphicon glyphicon-ok');
                textNode.textContent = ' Submitted';
                this.hidePasswordRecoveryWarning();
                setTimeout(() => {
                  this.promptModal.hide();
                }, 5000);
              },
              error: (xhr) => {
                let errorMessage = 'An unknown error occurred.';
                if (xhr.responseJSON && xhr.responseJSON.error) {
                  errorMessage = xhr.responseJSON.error;
                } else if (xhr.responseText) {
                  errorMessage = xhr.responseText;
                }
                errorContainer.html(errorMessage).show();
                saveButton.prop('disabled', false);
                icon.remove();
                textNode.textContent = ' Retry';
              }
            });
          }
        }
      ]
    });
  }
}

