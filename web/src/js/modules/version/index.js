export function checkNewVersion() {
  // Skip if already checked this session
  if (sessionStorage.getItem('pat_version_checked') === 'true') {
    console.log('Skipping version check - already checked this session');
    return;
  }
  // Check if within 72h reminder period
  const lastCheck = parseInt(localStorage.getItem('pat_version_check_time') || '0');
  const now = new Date().getTime();
  if (now - lastCheck < 72 * 60 * 60 * 1000) {
    console.log('Skipping version check - reminder snoozed');
    return;
  }

  $.ajax({
    url: '/api/new-release-check',
    method: 'GET',
    success: function(data, textStatus, xhr) {
      // Mark as checked for this session
      sessionStorage.setItem('pat_version_checked', 'true');

      // If status is 204, there's no new version
      if (xhr.status === 204) {
        console.log('No new version available');
        return;
      }

      // Skip if this version was ignored
      const ignoredVersion = localStorage.getItem('pat_ignored_version');
      if (data.version === ignoredVersion) {
        console.log(`Skipping version prompt - version ignored (${ignoredVersion})`);
        return;
      }

      // Log success and show prompt modal
      console.log('Successfully checked for new version');
      const modal = $('#promptModal');
      const modalBody = modal.find('.modal-body');
      const modalFooter = modal.find('.modal-footer');

      $('#promptMessage').text('A new version of Pat is available!');

      modalBody.empty();
      modalBody.append($('<p>').html(`Version ${data.version} is now available 🎉`));
      modalBody.append($('<p>').html(`<a href="${data.release_url}" target="_blank">View release details</a>`));

      modalFooter.empty();
      modalFooter.append(
        $('<button>')
          .attr({
            type: 'button',
            class: 'btn btn-default pull-left'
          })
          .text('Ignore this version')
          .click(function() {
            localStorage.setItem('pat_ignored_version', data.version);
            modal.modal('hide');
          })
      );

      modalFooter.append(
        $('<button>')
          .attr({
            type: 'button',
            class: 'btn btn-default'
          })
          .text('Remind me later')
          .click(function() {
            localStorage.setItem('pat_version_check_time', new Date().getTime());
            modal.modal('hide');
          })
      );

      modalFooter.append(
        $('<button>')
          .attr({
            type: 'button',
            class: 'btn btn-primary'
          })
          .text('Download')
          .click(function() {
            window.open(data.release_url, '_blank');
            modal.modal('hide');
          })
      );

      modal.modal('show');
    },
    error: function(xhr, textStatus, errorThrown) {
      console.log('Version check failed:', textStatus, errorThrown);
    }
  });
}
