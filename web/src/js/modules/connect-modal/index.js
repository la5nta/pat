import URI from 'urijs';
import $ from 'jquery';
import { alert } from '../utils';
import { RmslistView } from './rmslist-view';

class ConnectModal {
  constructor(mycall) {
    this.mycall = mycall;
    this.initialized = false;
    this.connectAliases = {};
    this.rmslistView = new RmslistView();
  }

  init() {
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
    $('#connectURLInput').val(url);
  }

  buildConnectURL() {
    // Instead of building from scratch, we use the current URL as a starting
    // point to retain URI parts not supported by the modal. The unsupported
    // parts may originate from a connect alias or by manual edit of the URL
    // field.
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
    if ($('#freqInput').val() && $('#freqInput').parent().hasClass('has-success')) {
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
        this.onConnectInputChange();
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
      success: function(data) {
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
        select.val(selected).change();
      },
      complete: function(xhr) {
        select.attr('x-for-transport', transport);
        div.toggle(select.find('option').length > 0);
        select.prop('disabled', false);
        select.selectpicker('refresh');
      },
    });
  }

  updateConnectAliases() {
    $.getJSON('/api/connect_aliases', (data) => {
      this.connectAliases = data;

      const select = $('#aliasSelect');
      Object.keys(data).forEach(function(key) {
        select.append('<option>' + key + '</option>');
      });

      select.change(() => {
        $('#aliasSelect option:selected').each((i, E) => {
          const alias = $(E).text();
          const url = this.connectAliases[alias];
          this.setConnectValues(url);
          select.val('');
          select.selectpicker('refresh');
        });
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
