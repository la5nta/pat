import URI from 'urijs';
import $ from 'jquery';
import { alert } from '../utils';

let initialized = false;
let connectAliases;

export function initConnectModal() {
  $('#freqInput').on('focusin focusout', (e) => {
    // Disable the connect button while the user is editing the frequency value.
    //   We do this because we really don't want the user to hit the connect
    //   button until they know that the QSY command succeeded or failed.
    window.setTimeout(() => {
      $('#connect_btn').prop('disabled', e.type == 'focusin');
    }, 300);
  });
  $('#freqInput').change(() => {
    onConnectInputChange();
    onConnectFreqChange();
  });
  $('#bandwidthInput').change(onConnectBandwidthChange);
  $('#radioOnlyInput').change(onConnectInputChange);
  $('#addrInput').change(onConnectInputChange);
  $('#targetInput').change(onConnectInputChange);
  $('#connectRequestsInput').change(onConnectInputChange);
  $('#connectURLInput').change((e) => {
    setConnectValues($(e.target).val())
  });
  $('#updateRmslistButton').click((e) => {
    $(e.target).prop('disabled', true);
    updateRmslist(true);
  });

  $('#modeSearchSelect').change(updateRmslist);
  $('#bandSearchSelect').change(updateRmslist);

  $('#transportSelect').change(function(e) {
    // Clear existing options
    $('#bandwidthInput').val('').change();
    $('#addrInput').val('').change();
    $('#freqInput').val('').change();
    $('#connectRequestsInput').val('').change();
    setConnectURL('');

    // Refresh views
    refreshExtraInputGroups();
    onConnectInputChange();
    onConnectFreqChange();

    // Update rmslist view
    switch ($(e.target).val()) {
      case 'ardop':
      case 'pactor':
      case 'varafm':
      case 'varahf':
        $('#modeSearchSelect').val($(e.target).val());
        break;
      case 'ax25':
      case 'ax25+linux':
      case 'ax25+agwpe':
      case 'ax25+serial-tnc':
        $('#modeSearchSelect').val('packet');
        break;
      default:
        return;
    }
    $('#modeSearchSelect').selectpicker('refresh');
    updateRmslist();
  });
  let url = localStorage.getItem('pat_connect_url');
  if (url != null) {
    setConnectValues(url);
  }
  refreshExtraInputGroups();
  initialized = true;

  updateConnectAliases();
  updateRmslist();
}

function getConnectURL() {
  return $('#connectURLInput').val();
}

function setConnectURL(url) {
  $('#connectURLInput').val(url);
}

function buildConnectURL() {
  // Instead of building from scratch, we use the current URL as a starting
  // point to retain URI parts not supported by the modal. The unsupported
  // parts may originate from a connect alias or by manual edit of the URL
  // field.
  let transport = $('#transportSelect').val();
  var current = URI(getConnectURL());
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

function onConnectFreqChange() {
  if (!initialized) {
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
      onConnectInputChange();
    }, // This removes freq= from URL in case of failure
  });
}

function onConnectBandwidthChange() {
  const input = $(this);
  console.log("connect bandwidth change " + input.val());
  input.attr('x-value', input.val());
  if (input.val() === '') {
    input.removeAttr('x-value');
  }
  onConnectInputChange();
}

function onConnectInputChange() {
  setConnectURL(buildConnectURL());
}

function refreshExtraInputGroups() {
  const transport = $('#transportSelect').val();
  populateBandwidths(transport);
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

function populateBandwidths(transport) {
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

function updateRmslist(forceDownload) {
  let tbody = $('#rmslist tbody');
  let params = {
    mode: $('#modeSearchSelect').val(),
    band: $('#bandSearchSelect').val(),
    'force-download': forceDownload === true,
  };
  $.ajax({
    method: 'GET',
    url: '/api/rmslist',
    dataType: 'json',
    data: params,
    success: function(data) {
      tbody.empty();
      data.forEach((rms) => {
        let tr = $('<tr>')
          .append($('<td class="text-left">').text(rms.callsign))
          .append($('<td class="text-left">').text(rms.distance.toFixed(0) + ' km'))
          .append($('<td class="text-left">').text(rms.modes))
          .append($('<td class="text-right">').text(rms.dial.desc));
        tr.click((e) => {
          tbody.find('.active').removeClass('active');
          tr.addClass('active');
          setConnectValues(rms.url);
        });
        tbody.append(tr);
      });
    },
  });
}

function updateConnectAliases() {
  $.getJSON('/api/connect_aliases', function(data) {
    connectAliases = data;

    const select = $('#aliasSelect');
    Object.keys(data).forEach(function(key) {
      select.append('<option>' + key + '</option>');
    });

    select.change(function() {
      $('#aliasSelect option:selected').each(function() {
        const alias = $(this).text();
        const url = connectAliases[$(this).text()];
        setConnectValues(url);
        select.val('');
        select.selectpicker('refresh');
      });
    });
    select.selectpicker('refresh');
  });
}

function setConnectValues(url) {
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

  refreshExtraInputGroups();
  onConnectInputChange();
  onConnectFreqChange();
  setConnectURL(url);
}

export function connect(evt) {
  const url = getConnectURL();
  localStorage.setItem('pat_connect_url', url);
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
