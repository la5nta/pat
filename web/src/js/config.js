import 'jquery';
import 'bootstrap/dist/js/bootstrap';
import 'bootstrap-select';
import 'bootstrap-tokenfield';

$(document).ready(function() {
  // Function to enforce minimum beacon interval
  function enforceMinBeaconInterval(input) {
    const value = parseInt(input.val(), 10);
    if (value > 0 && value < 10) {
      input.val(10);
    }
  }

  // Add blur handlers for beacon intervals
  $('#ardop_beacon_interval, #ax25_beacon_interval').on('blur', function() {
    enforceMinBeaconInterval($(this));
  });

  $('#mycall').on('blur', function() {
    var icon = $('#mycall-status');
    icon.empty();
    var callsign = $(this).val();
    if (callsign.length < 3) {
      icon.css('visibility', 'hidden');
      return;
    }
    icon.css('visibility', 'visible');
    icon.append($('<span>').addClass('glyphicon glyphicon-refresh icon-spin'));
    $.ajax({
      url: '/api/winlink-account/registration?callsign=' + callsign,
      type: 'GET',
      dataType: 'json',
      timeout: 10000,
      success: function(data) {
        icon.empty();
        if (data.exists) {
          icon.append($('<span>').addClass('glyphicon glyphicon-ok text-success').attr('title', 'Winlink account exists'));
          $('#create-account-prompt').hide();
        } else {
          icon.append($('<span>').addClass('glyphicon glyphicon-remove text-danger').attr('title', 'Winlink account does not exist'));
          $('#create-account-prompt').show();
        }
      },
      error: function() {
        icon.empty();
        icon.append($('<span>').addClass('glyphicon glyphicon-warning-sign text-warning').attr('title', 'Unable to verify Winlink account status'));
        $('#create-account-prompt').hide();
      }
    });
  });

  // Modal handling
  $('#create-account-link').click(function(e) {
    e.preventDefault();
    $('#modal-mycall').val($('#mycall').val());
    // Reset modal to step 1
    navigateToStep(1);
    $('.breadcrumb-step').removeClass('completed');
    $('#createAccountModal').modal('show');
  });

  function navigateToStep(step) {
    // Update breadcrumbs
    $('.breadcrumb-step').removeClass('active');
    $('.breadcrumb-step[data-step="' + step + '"]').addClass('active');

    for (let i = 1; i < step; i++) {
      $('.breadcrumb-step[data-step="' + i + '"]').addClass('completed');
    }
    for (let i = step; i <= 4; i++) {
      $('.breadcrumb-step[data-step="' + i + '"]').removeClass('completed');
    }


    // Show/hide panes
    $('.tab-pane').hide();
    $('#step' + step).show();
  }

  function validatePassword() {
    var password = $('#modal-password').val();
    var verifyPassword = $('#modal-password-verify').val();
    var passwordStatus = $('#password-status');
    var verifyStatus = $('#password-verify-status');
    var nextBtn = $('#next-step2');

    var isLengthValid = password.length >= 6 && password.length <= 12;
    var doPasswordsMatch = password === verifyPassword;

    // Length validation
    passwordStatus.empty();
    if (isLengthValid) {
      passwordStatus.append($('<span>').addClass('glyphicon glyphicon-ok text-success'));
    } else {
      passwordStatus.append($('<span>').addClass('glyphicon glyphicon-remove text-danger'));
    }

    // Match validation
    verifyStatus.empty();
    if (verifyPassword.length > 0) {
      if (doPasswordsMatch) {
        verifyStatus.append($('<span>').addClass('glyphicon glyphicon-ok text-success'));
      } else {
        verifyStatus.append($('<span>').addClass('glyphicon glyphicon-remove text-danger'));
      }
    }

    // Enable/disable next button
    if (isLengthValid && doPasswordsMatch) {
      nextBtn.prop('disabled', false);
    } else {
      nextBtn.prop('disabled', true);
    }
  }

  $('#modal-password, #modal-password-verify').on('input', validatePassword);
  $('#modal-email').on('input', function() {
    var email = $(this).val();
    var emailStatus = $('#email-status');
    var nextBtn = $('#next-step3');

    // Simple email regex
    var emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

    emailStatus.empty();
    if (email.length === 0) {
      nextBtn.prop('disabled', false);
      return;
    }

    if (emailRegex.test(email)) {
      emailStatus.append($('<span>').addClass('glyphicon glyphicon-ok text-success'));
      nextBtn.prop('disabled', false);
    } else {
      emailStatus.append($('<span>').addClass('glyphicon glyphicon-remove text-danger'));
      nextBtn.prop('disabled', true);
    }
  });

  $('#next-step1').click(function() { navigateToStep(2); });
  $('#prev-step2').click(function() { navigateToStep(1); });
  $('#next-step2').click(function() { navigateToStep(3); });
  $('#prev-step3').click(function() { navigateToStep(2); });
  $('#next-step3').click(function() { navigateToStep(4); });
  $('#prev-step4').click(function() { navigateToStep(3); });

  $('#consent-checkbox').on('change', function() {
    $('#finish-creation').prop('disabled', !$(this).is(':checked'));
  });

  $('#finish-creation').click(function() {
    var callsign = $('#modal-mycall').val();
    var password = $('#modal-password').val();
    var email = $('#modal-email').val();

    var btn = $(this);
    btn.prop('disabled', true).html('<span class="glyphicon glyphicon-hourglass" style="margin-right: 5px;"></span> Creating...');

    $.ajax({
      url: '/api/winlink-account/registration',
      type: 'POST',
      contentType: 'application/json',
      data: JSON.stringify({
        callsign: callsign,
        password: password,
        password_recovery_email: email
      }),
      success: function() {
        btn.html('<span class="glyphicon glyphicon-ok" style="margin-right: 5px;"></span> Created');
        $('#createAccountModal').modal('hide');
        // Update icon to success since we just created it.
        var icon = $('#mycall-status');
        icon.empty();
        icon.append($('<span>').addClass('glyphicon glyphicon-ok text-success').attr('title', 'Winlink account created'));
        $('#secure_login_password').val(password);
        var prompt = $('#create-account-prompt');
        prompt.removeClass('alert-warning').addClass('alert-success').empty().append(
          $('<span>').text('Account ' + callsign + ' created.')
        ).show();
      },
      error: function(xhr) {
        btn.prop('disabled', false).html('Create Account');
        alert('Failed to create account: ' + ((xhr.responseJSON && xhr.responseJSON.error) || xhr.statusText));
      }
    });
  });

  $('#toggle-password').click(function() {
    var passwordField = $('#secure_login_password');
    var passwordFieldType = passwordField.attr('type');
    var icon = $(this).find('span');

    if (passwordFieldType === 'password') {
      passwordField.attr('type', 'text');
      icon.removeClass('glyphicon-eye-open').addClass('glyphicon-eye-close');
    } else {
      passwordField.attr('type', 'password');
      icon.removeClass('glyphicon-eye-close').addClass('glyphicon-eye-open');
    }
  });

  // Initialize Bootstrap Select components and tokenfield
  $('#ardop_arq_bandwidth, #vara_hf_bandwidth, #vara_fm_bandwidth').selectpicker();
  const tokenfieldConfig = {
    delimiter: [',', ';', ' '],
    createTokensOnBlur: true,
  };
  $('#auxiliary_addresses').tokenfield(tokenfieldConfig);
  // Load current config
  let originalConfig;
  $.ajax({
    url: '/api/config',
    type: 'GET',
    dataType: 'json',
    success: function(config) {
      originalConfig = JSON.parse(JSON.stringify(config)); // Deep clone
      $('#mycall').val(config.mycall || '').trigger('blur');
      $('#locator').val(config.locator || '');
      $('#auto_download_limit').val(typeof config.auto_download_size_limit === 'number' ? config.auto_download_size_limit : -1);
      $('#secure_login_password').val(config.secure_login_password ? '[REDACTED]' : '')
        .on('focus', function() {
          if (originalConfig.secure_login_password && $(this).val() === '[REDACTED]') {
            $(this).val('');
          }
        })
        .on('blur', function() {
          if (originalConfig.secure_login_password && $(this).val() === '') {
            $(this).val('[REDACTED]');
          }
        });

      // Populate auxiliary addresses as direct strings
      const auxAddrs = (config.auxiliary_addresses || [])
        .filter(addr => addr && addr.trim()) // Filter empty/whitespace strings
        .map(addr => ({ value: addr, label: addr }));
      $('#auxiliary_addresses').tokenfield('setTokens', auxAddrs);

      // Populate rig selects
      // Initialize rig selects with any existing config
      updateRigSelects();

      // Populate transport configs
      $('#ardop_addr').val((config.ardop && config.ardop.addr) || '');
      // Convert bandwidth object to string format
      $('#ardop_connect_requests').val((config.ardop && config.ardop.connect_requests) || '');
      const arqBW = config.ardop && config.ardop.arq_bandwidth;
      const bwValue = arqBW ? `${arqBW.Max}${arqBW.Forced ? 'FORCED' : 'MAX'}` : '';
      $('#ardop_arq_bandwidth').val(bwValue).selectpicker('refresh');
      $('#ardop_cwid_enabled').prop('checked', (config.ardop && config.ardop.cwid_enabled) || false);
      $('#ardop_beacon_interval').val((config.ardop && config.ardop.beacon_interval) || '');
      $('#pactor_path').val((config.pactor && config.pactor.path) || '');
      $('#pactor_baudrate').val((config.pactor && config.pactor.baudrate) || '');
      $('#vara_hf_addr').val((config.varahf && config.varahf.addr) || '');
      $('#vara_hf_bandwidth').val((config.varahf && config.varahf.bandwidth && config.varahf.bandwidth.toString()) || '').selectpicker('refresh');
      $('#vara_fm_addr').val((config.varafm && config.varafm.addr) || '');
      $('#vara_fm_bandwidth').val((config.varafm && config.varafm.bandwidth && config.varafm.bandwidth.toString()) || '');

      // Populate AX25 config
      // Populate transport rig selections
      $('#ardop_rig').val((config.ardop && config.ardop.rig) || '');
      $('#ardop_ptt_ctrl').prop('checked', (config.ardop && config.ardop.ptt_ctrl) || false);
      $('#ax25_rig').val((config.ax25 && config.ax25.rig) || '');
      $('#ax25_ptt_ctrl').prop('checked', (config.ax25 && config.ax25.ptt_ctrl) || false);
      $('#pactor_rig').val((config.pactor && config.pactor.rig) || '');
      $('#pactor_ptt_ctrl').prop('checked', (config.pactor && config.pactor.ptt_ctrl) || false);
      $('#vara_hf_rig').val((config.varahf && config.varahf.rig) || '');
      $('#vara_hf_ptt_ctrl').prop('checked', (config.varahf && config.varahf.ptt_ctrl) || false);
      $('#vara_fm_rig').val((config.varafm && config.varafm.rig) || '');
      $('#vara_fm_ptt_ctrl').prop('checked', (config.varafm && config.varafm.ptt_ctrl) || false);
      $('#telnet_listen_addr').val((config.telnet && config.telnet.listen_addr) || '');
      $('#telnet_password').val((config.telnet && config.telnet.password) || '');
      $('#vara_hf_rig').val((config.varahf && config.varahf.rig) || '');
      $('#vara_fm_rig').val((config.varafm && config.varafm.rig) || '');

      // Set listen methods checkboxes
      const listenMethods = config.listen || [];
      $('input[name="listen_methods[]"]').each(function() {
        $(this).prop('checked', listenMethods.includes($(this).val()));
      });

      const ax25Config = config.ax25 || {};
      // Initialize all engine configs as collapsed first
      $('.ax25-engine-config .panel-collapse').collapse('hide');
      // Then show the selected one
      $('#ax25_engine').val(ax25Config.engine || 'linux').trigger('change');
      $('#ax25_linux_port').val((config.ax25linux && config.ax25linux.port) || '');
      $('#agwpe_addr').val((config.agwpe && config.agwpe.addr) || '');
      $('#agwpe_radio_port').val((config.agwpe && config.agwpe.radio_port) || 0);
      $('#serial_tnc_path').val((config.serial_tnc && config.serial_tnc.path) || '');
      $('#serial_tnc_baud').val((config.serial_tnc && config.serial_tnc.serial_baud) || 9600);
      $('#serial_tnc_type').val((config.serial_tnc && config.serial_tnc.type) || 'kenwood');
      $('#serial_tnc_hbaud').val((config.serial_tnc && config.serial_tnc.hbaud) || 1200);
      $('#ax25_beacon_interval').val((config.ax25 && config.ax25.beacon && config.ax25.beacon.every) || '');
      $('#ax25_beacon_message').val((config.ax25 && config.ax25.beacon && config.ax25.beacon.message) || '');
      $('#ax25_beacon_dest').val((config.ax25 && config.ax25.beacon && config.ax25.beacon.destination) || '');

      // Populate GPSd config
      $('#gpsd_enable_http').prop('checked', (config.gpsd && config.gpsd.enable_http) || false);
      $('#gpsd_allow_forms').prop('checked', (config.gpsd && config.gpsd.allow_forms) || false);
      $('#gpsd_use_server_time').prop('checked', (config.gpsd && config.gpsd.use_server_time) || false);
      $('#gpsd_addr').val((config.gpsd && config.gpsd.addr) || '');

      // Populate Hamlib rigs
      const rigs = config.hamlib_rigs || {};
      const rigsContainer = $('#rigsContainer');
      const rigTemplate = $('.rig-row').first().clone();
      rigsContainer.empty();

      Object.entries(rigs).forEach(([name, rig]) => {
        const row = rigTemplate.clone();
        const nameInput = row.find('.rig-name').val(name);
        nameInput.on('input', updateRigSelects); // Add input listener
        row.find('.rig-network').val(rig.network || '');
        row.find('.rig-address').val(rig.address || '');
        rigsContainer.append(row);
      });

      if (Object.keys(rigs).length === 0) {
        rigsContainer.append(rigTemplate.clone());
      }

      updateRigSelects(); // Refresh rig dropdowns with loaded data

      // Populate connect aliases
      const aliases = config.connect_aliases || {};
      const container = $('#aliasesContainer');
      const templateRow = $('.alias-row').first().clone(); // Cache template before emptying
      container.empty();

      Object.entries(aliases).forEach(([key, value]) => {
        const row = templateRow.clone();
        row.find('.alias-key').val(key);
        row.find('.alias-value').val(value);
        container.append(row);
      });

      // Always add at least one empty row
      if (Object.keys(aliases).length === 0) {
        container.append(templateRow.clone());
      }
    },
    error: function(xhr) {
      showError('Failed to load config: ' + xhr.responseText);
    }
  });

  // Handle form submission
  $('#configForm').submit(function(e) {
    e.preventDefault();

    // Create working copy from original
    const updatedConfig = JSON.parse(JSON.stringify(originalConfig));

    // Update listen methods from checkboxes
    updatedConfig.listen = $('input[name="listen_methods[]"]:checked').map(function() {
      return $(this).val();
    }).get();

    // Update GUI-managed fields
    updatedConfig.mycall = $('#mycall').val();
    updatedConfig.locator = $('#locator').val();
    updatedConfig.secure_login_password = $('#secure_login_password').val() || originalConfig.secure_login_password;
    updatedConfig.auto_download_size_limit = $('#auto_download_limit').val() === '' ? -1 : parseInt($('#auto_download_limit').val());

    // Auxiliary Addresses - store as direct strings
    updatedConfig.auxiliary_addresses = $('#auxiliary_addresses').tokenfield('getTokens')
      .filter(t => t.value.trim()) // Filter out empty tokens
      .map(token => token.value); // Store the full string value

    // Update transport configs with spread operators
    updatedConfig.ardop = {
      ...originalConfig.ardop,
      addr: $('#ardop_addr').val(),
      connect_requests: parseInt($('#ardop_connect_requests').val(), 10) || undefined,
      arq_bandwidth: (() => {
        const val = $('#ardop_arq_bandwidth').val();
        if (!val) return {};
        const match = val.match(/(\d+)(MAX|FORCED)/);
        return match ? {
          Max: parseInt(match[1], 10),
          Forced: match[2] === 'FORCED'
        } : {};
      })(),
      cwid_enabled: $('#ardop_cwid_enabled').is(':checked'),
      rig: $('#ardop_rig').val() || '',
      ptt_ctrl: $('#ardop_ptt_ctrl').is(':checked'),
      beacon_interval: parseInt($('#ardop_beacon_interval').val(), 10) || 0
    };
    // Merge pactor config with existing values
    updatedConfig.pactor = {
      ...originalConfig.pactor,
      path: $('#pactor_path').val(),
      baudrate: parseInt($('#pactor_baudrate').val(), 10),
      rig: $('#pactor_rig').val() || '',
      ptt_ctrl: $('#pactor_ptt_ctrl').is(':checked')
    };
    // Merge varahf config with existing values
    updatedConfig.varahf = {
      ...originalConfig.varahf,
      addr: $('#vara_hf_addr').val(),
      bandwidth: parseInt($('#vara_hf_bandwidth').val(), 10),
      rig: $('#vara_hf_rig').val() || '',
      ptt_ctrl: $('#vara_hf_ptt_ctrl').is(':checked')
    };
    // Merge varafm config with existing values
    updatedConfig.varafm = {
      ...originalConfig.varafm,
      addr: $('#vara_fm_addr').val(),
      rig: $('#vara_fm_rig').val() || '',
      ptt_ctrl: $('#vara_fm_ptt_ctrl').is(':checked')
    };
    // Merge telnet config with existing values
    updatedConfig.telnet = {
      ...originalConfig.telnet,
      listen_addr: $('#telnet_listen_addr').val(),
      password: $('#telnet_password').val()
    };
    updatedConfig.ax25 = {
      ...originalConfig.ax25,
      engine: $('#ax25_engine').val(),
      rig: $('#ax25_rig').val() || '',
      ptt_ctrl: $('#ax25_ptt_ctrl').is(':checked'),
      beacon: {
        ...(originalConfig.ax25 && originalConfig.ax25.beacon) || {}, // Preserve existing beacon fields
        every: parseInt($('#ax25_beacon_interval').val(), 10),
        message: $('#ax25_beacon_message').val(),
        destination: $('#ax25_beacon_dest').val()
      }
    };
    // Merge ax25linux config with existing values
    updatedConfig.ax25linux = {
      ...originalConfig.ax25linux,
      port: $('#ax25_linux_port').val()
    };
    // Merge agwpe config with existing values
    updatedConfig.agwpe = {
      ...originalConfig.agwpe,
      addr: $('#agwpe_addr').val(),
      radio_port: parseInt($('#agwpe_radio_port').val(), 10)
    };
    // Merge serial_tnc config with existing values
    updatedConfig.serial_tnc = {
      ...originalConfig.serial_tnc,
      path: $('#serial_tnc_path').val(),
      serial_baud: parseInt($('#serial_tnc_baud').val(), 10),
      type: $('#serial_tnc_type').val(),
      hbaud: parseInt($('#serial_tnc_hbaud').val(), 10)
    };
    updatedConfig.gpsd = {
      ...originalConfig.gpsd,
      enable_http: $('#gpsd_enable_http').is(':checked'),
      allow_forms: $('#gpsd_allow_forms').is(':checked'),
      use_server_time: $('#gpsd_use_server_time').is(':checked'),
      addr: $('#gpsd_addr').val()
    };


    // Handle collection fields (complete replacement)
    updatedConfig.hamlib_rigs = {};
    $('.rig-row').each(function() {
      const name = $(this).find('.rig-name').val();
      const network = $(this).find('.rig-network').val();
      const address = $(this).find('.rig-address').val();

      if (name && address) {
        updatedConfig.hamlib_rigs[name] = {
          address: address,
          network: network,
          VFO: $(this).find('.rig-vfo').val() || ''
        };
      }
    });

    updatedConfig.connect_aliases = {};
    $('.alias-row').each(function() {
      const key = $(this).find('.alias-key').val();
      const value = $(this).find('.alias-value').val();
      if (key && value) {
        updatedConfig.connect_aliases[key] = value;
      }
    });

    updatedConfig.schedule = {};
    $('.schedule-row').each(function() {
      const expr = $(this).find('.schedule-expr').val();
      const cmd = $(this).find('.schedule-cmd').val();
      if (expr && cmd) {
        updatedConfig.schedule[expr] = cmd;
      }
    });

    $.ajax({
      url: '/api/config',
      type: 'PUT',
      contentType: 'application/json',
      data: JSON.stringify(updatedConfig),
      success: function() {
        originalConfig = JSON.parse(JSON.stringify(updatedConfig)); // Update base config
        showSuccess('Configuration saved successfully');
        $('#restartNotice').modal('show');
        // Reset restart status when showing the modal
        $('#restartStatus').hide().empty();
        $('#restartNow').prop('disabled', false).html('<span class="glyphicon glyphicon-refresh" style="margin-right: 5px;"></span> Restart Now');
      },
      error: function(xhr) {
        showError('Save failed: ' + ((xhr.responseJSON && xhr.responseJSON.error) || xhr.statusText));
      }
    });
  });

  $('#restartNow').click(function() {
    const btn = $(this);
    const statusDiv = $('#restartStatus');

    btn.prop('disabled', true).html('<span class="glyphicon glyphicon-hourglass" style="margin-right: 5px;"></span> Restarting...');
    statusDiv.hide().empty();

    $.ajax({
      url: '/api/reload',
      type: 'POST',
      success: function() {
        setTimeout(function() {
          let attempts = 0;
          const maxAttempts = 30; // 3 seconds
          const interval = setInterval(function() {
            $.ajax({
              url: '/api/status',
              type: 'GET',
              success: function() {
                clearInterval(interval);
                btn.html('<span class="glyphicon glyphicon-ok" style="margin-right: 5px;"></span> Restarted');
                statusDiv.removeClass('alert-danger').addClass('alert alert-success').text('Restart successful!').show();
              },
              error: function() {
                attempts++;
                if (attempts >= maxAttempts) {
                  clearInterval(interval);
                  btn.prop('disabled', false).html('<span class="glyphicon glyphicon-refresh" style="margin-right: 5px;"></span> Restart Now');
                  statusDiv.removeClass('alert-success').addClass('alert alert-danger').text('Restart timed out. Please check the application logs.').show();
                }
              }
            });
          }, 100);
        }, 1000); // Wait 1s before starting to poll
      },
      error: function(xhr) {
        btn.prop('disabled', false).html('<span class="glyphicon glyphicon-refresh" style="margin-right: 5px;"></span> Restart Now');
        statusDiv.removeClass('alert-success').addClass('alert alert-danger').text('Restart failed: ' + xhr.responseText).show();
      }
    });
  });

  function showSuccess(msg) {
    $('#statusMessage').removeClass('text-danger').addClass('text-success').text(msg);
  }

  function showError(msg) {
    $('#statusMessage').removeClass('text-success').addClass('text-danger').text(msg);
  }

  // Handle alias add/delete
  // Handle AX25 engine change
  $('#ax25_engine').on('change', function() {
    const selectedEngine = $(this).val();
    // Hide all engine configs first
    $('.ax25-engine-config').each(function() {
      const configDiv = $(this).find('.panel-collapse');
      if ($(this).data('engine') === selectedEngine) {
        configDiv.collapse('show');
      } else {
        configDiv.collapse('hide');
      }
    });
  }).trigger('change');

  // Handle rig add/delete
  $('#rigsContainer').on('click', '.delete-rig', function() {
    $(this).closest('.rig-row').remove();
  });

  function updateRigSelects() {
    const rigNames = [];
    $('.rig-row .rig-name').each(function() {
      const name = $(this).val();
      if (name) rigNames.push(name);
    });

    $('.rig-select').each(function() {
      $(this).empty().append($('<option>').val('').text('None'));
      rigNames.forEach(name => {
        $(this).append($('<option>').val(name).text(name));
      });
    });
  }

  $('#addRig').click(function() {
    const newRow = $('.rig-row').first().clone();
    newRow.find('input').val('');
    newRow.find('.rig-name').on('input', updateRigSelects); // Add input listener
    $('#rigsContainer').append(newRow);
    updateRigSelects(); // Refresh selects after adding
  });

  $('#aliasesContainer').on('click', '.delete-alias', function() {
    $(this).closest('.alias-row').remove();
  });

  $('#addAlias').click(function() {
    const newRow = $('.alias-row').first().clone();
    newRow.find('input').val('');
    $('#aliasesContainer').append(newRow);
  });
});
