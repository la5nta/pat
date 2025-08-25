import URI from 'urijs';
import $ from 'jquery';
import { alert } from '../utils';
import { RmslistView } from './rmslist-view';
import { PromptModal } from '../prompt';

class ConnectModal {
  constructor(mycall) {
    this.mycall = mycall;
    this.initialized = false;
    this.connectAliases = {};
    this.rmslistView = new RmslistView();
    this.preserveAliasSelection = false;
    this.promptModal = new PromptModal();
  }

  init() {
    this.promptModal.init();

    $('#connect_btn').click(() => this.connect());
    $('#connectForm input').keypress((e) => {
      if (e.which == 13) {
        this.connect();
        return false;
      }
    });

    $('#freqInput').on('focusin focusout', (e) => {
      // Disable the connect button while the user is editing the frequency value.
      //   We do this because we really don't want the user to hit the connect
      //   button until they know that the QSY command succeeded or failed.
      window.setTimeout(() => {
        $('#connect_btn').prop('disabled', e.type == 'focusin');
      }, 300);
    });
    $('#freqInput').change(() => {
      this.onConnectInputChange();
      this.onConnectFreqChange();
    });
    $('#bandwidthInput').change((e) => this.onConnectBandwidthChange(e));
    $('#radioOnlyInput').change(() => this.onConnectInputChange());
    $('#addrInput').change(() => this.onConnectInputChange());
    $('#targetInput').change(() => this.onConnectInputChange());
    $('#connectRequestsInput').change(() => this.onConnectInputChange());
    $('#connectURLInput').change((e) => {
      this.setConnectValues($(e.target).val())
    });

    // Alias action button handler (add/delete)
    $('#aliasActionBtn').click(() => {
      const selectedAlias = $('#aliasSelect').val();
      if (selectedAlias && selectedAlias !== '') {
        // Delete mode
        this.deleteAlias(selectedAlias);
      } else {
        // Add mode
        this.saveAsNewAlias();
      }
    });

    // Initialize RMS list manager
    this.rmslistView.init();
    this.rmslistView.onRowClick = (url) => this.setConnectValues(url);

    $('#transportSelect').change((e) => {
      // Clear existing options
      $('#bandwidthInput').val('').change();
      $('#addrInput').val('').change();
      $('#freqInput').val('').change();
      $('#connectRequestsInput').val('').change();
      this.setConnectURL('');

      // Refresh views
      this.refreshExtraInputGroups();
      this.onConnectInputChange();
      this.onConnectFreqChange();

      // Update rmslist view
      this.rmslistView.onTransportChange($(e.target).val());
    });
    let url = localStorage.getItem(`pat_connect_url_${this.mycall}`);
    if (url != null) {
      this.setConnectValues(url);
    }
    this.refreshExtraInputGroups();
    this.initialized = true;

    this.updateConnectAliases();
    this.updateAliasActionButton();
    this._initConfigDefaults();
  }

  _initConfigDefaults() {
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

  getConnectURL() {
    return $('#connectURLInput').val();
  }

  setConnectURL(url) {
    $('#connectURLInput').val(decodeURIComponent(url));
  }

  buildConnectURL(options = {}) {
    // Instead of building from scratch, we use the current URL as a starting
    // point to retain URI parts not supported by the modal. The unsupported
    // parts may originate from a connect alias or by manual edit of the URL
    // field.
    const { preserveFreq = false } = options;

    let transport = $('#transportSelect').val();
    var current = URI(this.getConnectURL());
    var url;
    if (transport === 'telnet') {
      // Telnet is special cased, as the address contains more than hostname.
      url = URI(transport + "://" + $('#addrInput').val() + current.search());
    } else {
      url = current.protocol(transport).hostname($('#addrInput').val());
    }
    url = url.path($('#targetInput').val());
    if ($('#freqInput').val() && (preserveFreq || $('#freqInput').parent().hasClass('has-success'))) {
      url = url.setQuery("freq", $('#freqInput').val());
    } else {
      url = url.removeQuery("freq");
    }
    if ($('#bandwidthInput').val()) {
      url = url.setQuery("bw", $('#bandwidthInput').val());
    } else {
      url = url.removeQuery("bw");
    }
    if ($('#radioOnlyInput').is(':checked')) {
      url = url.setQuery("radio_only", "true");
    } else {
      url = url.removeQuery("radio_only");
    }
    if ($('#connectRequestsInput').val()) {
      url = url.setQuery('connect_requests', $('#connectRequestsInput').val());
    } else {
      url = url.removeQuery('connect_requests');
    }
    return url.build();
  }

  onConnectFreqChange() {
    if (!this.initialized) {
      console.log('Skipping QSY during initialization');
      return;
    }

    $('#qsyWarning').empty().attr('hidden', true);

    const freqInput = $('#freqInput');
    freqInput.css('text-decoration', 'none currentcolor solid');

    const inputGroup = freqInput.parent();
    ['has-error', 'has-success', 'has-warning'].forEach((v) => {
      inputGroup.removeClass(v);
    });
    inputGroup.tooltip('destroy');

    const data = {
      transport: $('#transportSelect').val(),
      freq: new Number(freqInput.val()),
    };
    if (data.freq == 0) {
      return;
    }

    console.log('QSY: ' + JSON.stringify(data));
    $.ajax({
      method: 'POST',
      url: '/api/qsy',
      data: JSON.stringify(data),
      contentType: 'application/json',
      success: () => {
        inputGroup.addClass('has-success');
      },
      error: (xhr) => {
        freqInput.css('text-decoration', 'line-through');
        if (xhr.status == 503) {
          // The feature is unavailable
          inputGroup
            .attr('data-toggle', 'tooltip')
            .attr(
              'title',
              'Rigcontrol is not configured for the selected transport. Set radio frequency manually.'
            )
            .tooltip('fixTitle');
        } else {
          // An unexpected error occured
          [inputGroup, $('#qsyWarning')].forEach((e) => {
            e.attr('data-toggle', 'tooltip')
              .attr(
                'title',
                'Could not set radio frequency. See log output for more details and/or set the frequency manually.'
              )
              .tooltip('fixTitle');
          });
          inputGroup.addClass('has-error');
          $('#qsyWarning')
            .html('<span class="glyphicon glyphicon-warning-sign"></span> QSY failure')
            .attr('hidden', false);
        }
      },
      complete: () => {
        // Update URL after QSY operation, but don't clear alias selection
        this.withPreservedAliasSelection(() => {
          this.onConnectInputChange();
        });
      }, // This removes freq= from URL in case of failure
    });
  }

  onConnectBandwidthChange(e) {
    const input = $(e.target);
    console.log("connect bandwidth change " + input.val());
    input.attr('x-value', input.val());
    if (input.val() === '') {
      input.removeAttr('x-value');
    }
    this.onConnectInputChange();
  }

  onConnectInputChange() {
    this.setConnectURL(this.buildConnectURL());

    // Clear alias selection when user modifies inputs (unless we want to preserve it)
    if (!this.preserveAliasSelection) {
      $('#aliasSelect').val('').selectpicker('refresh');
      this.updateAliasActionButton();
    }
  }

  updateAliasActionButton() {
    const selectedAlias = $('#aliasSelect').val();
    const button = $('#aliasActionBtn');
    const icon = button.find('.glyphicon');

    if (selectedAlias && selectedAlias !== '') {
      // Delete mode - show trash icon
      icon.removeClass('glyphicon-plus').addClass('glyphicon-trash');
      button.attr('title', 'Delete selected alias');
      button.show();
    } else {
      // Add mode - show plus icon
      icon.removeClass('glyphicon-trash').addClass('glyphicon-plus');
      button.attr('title', 'Save current configuration as new alias');
      button.show();
    }
  }

  withPreservedAliasSelection(callback) {
    this.preserveAliasSelection = true;
    callback();
    this.preserveAliasSelection = false;
  }

  deleteAlias(aliasName) {
    this.promptModal.showCustom({
      message: `Are you sure you want to delete the alias "${aliasName}"?`,
      buttons: [
        {
          text: 'Cancel',
          class: 'btn-default',
          onClick: () => { } // Just close modal
        },
        {
          text: 'Delete',
          class: 'btn-danger',
          onClick: () => {
            $.ajax({
              method: 'DELETE',
              url: `/api/config/connect_aliases/${encodeURIComponent(aliasName)}`,
              success: () => {
                // Remove from local cache and dropdown
                delete this.connectAliases[aliasName];
                $('#aliasSelect option').filter(function() {
                  return $(this).text() === aliasName;
                }).remove();

                // Clear selection and update button
                $('#aliasSelect').val('').selectpicker('refresh');
                this.updateAliasActionButton();
              },
              error: (xhr) => {
                console.error('Failed to delete alias:', xhr);
              }
            });
          }
        }
      ]
    });
  }

  validateAliasName(name) {
    // Check for empty or invalid characters
    if (!name || !/^[a-zA-Z0-9_.@-]+$/.test(name)) {
      return 'Alias name must contain only letters, numbers, dashes, underscores, dots, and @ symbols.';
    }

    // Check for duplicates
    if (this.connectAliases[name]) {
      return 'An alias with this name already exists.';
    }

    return null; // Valid
  }

  promptForAliasName() {
    return new Promise((resolve) => {
      const tryAgain = (errorMessage = null) => {
        // Create input field
        const inputId = 'aliasNameInput';
        const body = $('<div>');

        // Add description
        body.append(
          $('<p>').text('Create a new alias to save this connection configuration for quick access.')
        );

        // Add allowed characters info
        body.append(
          $('<p>').addClass('text-muted small').text('Allowed characters: letters, numbers, dashes (-), underscores (_), dots (.), and @ symbols')
        );

        // Add error message if any
        if (errorMessage) {
          body.append(
            $('<div>').addClass('alert alert-danger').text(errorMessage)
          );
        }

        // Add input field
        body.append(
          $('<input>').attr({
            type: 'text',
            id: inputId,
            class: 'form-control',
            placeholder: 'Enter alias name...',
            autocomplete: 'off'
          })
        );

        this.promptModal.showCustom({
          message: 'New Connection Alias',
          body: body,
          buttons: [
            {
              text: 'Cancel',
              class: 'btn-default',
              onClick: () => {
                resolve(null);
              }
            },
            {
              text: 'Save',
              class: 'btn-primary',
              onClick: () => {
                const aliasName = $(`#${inputId}`).val().trim();

                // Validate the name
                const validationError = this.validateAliasName(aliasName);
                if (validationError) {
                  // Show error and try again
                  this.promptModal.hide();
                  setTimeout(() => tryAgain(validationError), 100);
                  return;
                }

                resolve(aliasName);
              }
            }
          ]
        });

        // Focus input after modal is shown
        this.promptModal.modal.one('shown.bs.modal', () => {
          $(`#${inputId}`).focus();
        });
      };

      tryAgain();
    });
  }

  saveAsNewAlias() {
    this.promptForAliasName().then((aliasName) => {
      if (!aliasName) {
        // User cancelled, revert dropdown selection
        $('#aliasSelect').val('').selectpicker('refresh');
        return;
      }

      const connectURL = this.buildConnectURL({ preserveFreq: true }).toString();
      if (!connectURL) {
        alert('No connection URL to save. Please configure connection settings first.');
        $('#aliasSelect').val('').selectpicker('refresh');
        return;
      }

      $.ajax({
        method: 'PUT',
        url: `/api/config/connect_aliases/${encodeURIComponent(aliasName)}`,
        data: JSON.stringify(connectURL),
        contentType: 'application/json',
        success: () => {
          // Add to local cache
          this.connectAliases[aliasName] = connectURL;

          // Add to dropdown
          const option = $(`<option>${aliasName}</option>`);
          $('#aliasSelect').append(option);

          // Select the new alias
          $('#aliasSelect').val(aliasName).selectpicker('refresh');
          this.updateAliasActionButton();
        },
        error: (xhr) => {
          console.error('Failed to save alias:', xhr);
          alert('Failed to save alias. Please try again.');
          $('#aliasSelect').val('').selectpicker('refresh');
        }
      });
    });
  }

  refreshExtraInputGroups() {
    const transport = $('#transportSelect').val();
    this.populateBandwidths(transport);
    switch (transport) {
      case 'telnet':
        $('#freqInputDiv').hide();
        $('#addrInputDiv').show();
        $('#connectRequestsInputDiv').hide();
        break;
      case 'ardop':
        $('#addrInputDiv').hide();
        $('#freqInputDiv').show();
        $('#connectRequestsInputDiv').show();
        break;
      default:
        $('#addrInputDiv').hide();
        $('#freqInputDiv').show();
        $('#connectRequestsInputDiv').hide();
    }

    if (transport.startsWith('ax25')) {
      $('#radioOnlyInput')[0].checked = false;
      $('#radioOnlyInputDiv').hide();
    } else {
      $('#radioOnlyInputDiv').show();
    }
  }

  populateBandwidths(transport) {
    const select = $('#bandwidthInput');
    const div = $('#bandwidthInputDiv');
    var selected = select.attr('x-value');
    select.empty();
    select.prop('disabled', true);
    $.ajax({
      method: 'GET',
      url: `/api/bandwidths?mode=${transport}`,
      dataType: 'json',
      success: (data) => {
        if (data.bandwidths.length === 0) {
          return;
        }
        if (selected === undefined) {
          selected = data.default;
        }
        data.bandwidths.forEach((bw) => {
          const option = $(`<option value="${bw}">${bw}</option>`);
          option.prop('selected', bw === selected);
          select.append(option);
        });
        // Use programmatic wrapper for the change event
        this.withPreservedAliasSelection(() => {
          select.val(selected).change();
        });
      },
      complete: (xhr) => {
        select.attr('x-for-transport', transport);
        div.toggle(select.find('option').length > 0);
        select.prop('disabled', false);
        select.selectpicker('refresh');
      },
    });
  }

  updateConnectAliases() {
    $.getJSON('/api/config/connect_aliases', (data) => {
      this.connectAliases = data;

      const select = $('#aliasSelect');
      Object.keys(data).forEach(function(key) {
        select.append('<option>' + key + '</option>');
      });

      select.change(() => {
        const selectedAlias = select.val();
        const hasSelection = selectedAlias && selectedAlias !== '';
        this.updateAliasActionButton();

        if (hasSelection) {
          $('#aliasSelect option:selected').each((i, E) => {
            const alias = $(E).text();
            const url = this.connectAliases[alias];
            this.withPreservedAliasSelection(() => {
              this.setConnectValues(url);
            });
          });
        }
      });
      select.selectpicker('refresh');
    });
  }

  setConnectValues(url) {
    url = URI(url.toString());

    $('#transportSelect').val(url.protocol());
    $('#transportSelect').selectpicker('refresh');

    $('#targetInput').val(url.path().substr(1));

    const query = url.search(true);

    if (url.hasQuery('freq')) {
      $('#freqInput').val(query['freq']);
    } else {
      $('#freqInput').val('');
    }

    if (url.hasQuery('bw')) {
      $('#bandwidthInput').val(query['bw']).change();
      $('#bandwidthInput').attr('x-value', query['bw']); // Since the option might not be available yet.
    } else {
      $('#bandwidthInput').val('').change();
      $('#bandwidthInput').removeAttr('x-value');
    }

    if (url.hasQuery('radio_only')) {
      $('#radioOnlyInput')[0].checked = query['radio_only'];
    } else {
      $('#radioOnlyInput')[0].checked = false;
    }

    if (url.hasQuery('connect_requests')) {
      $('#connectRequestsInput').val(query['connect_requests']);
    }

    let usri = '';
    if (url.username()) {
      usri += url.username();
    }
    if (url.password()) {
      usri += ':' + url.password();
    }
    if (usri != '') {
      usri += '@';
    }
    $('#addrInput').val(usri + url.host());

    this.refreshExtraInputGroups();
    this.onConnectInputChange();
    this.onConnectFreqChange();
    this.setConnectURL(url);
  }

  toggle() {
    $('#connectModal').modal('toggle');
  }

  connect(evt) {
    const url = this.getConnectURL();
    localStorage.setItem(`pat_connect_url_${this.mycall}`, url);
    $('#connectModal').modal('hide');

    $.getJSON('/api/connect?url=' + encodeURIComponent(url), function(data) {
      if (data.NumReceived == 0) {
        window.setTimeout(function() {
          alert('No new messages.');
        }, 1000);
      }
    }).fail(function() {
      alert('Connect failed. See console for detailed information.');
    });
  }
}

export { ConnectModal };
