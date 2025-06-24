import { PromptModal } from '../prompt/index.js';

const promptModal = PromptModal.getInstance();

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

      promptModal.showCustom({
        message: 'A new version of Pat is available!',
        body: $('<div>')
          .append($('<p>').html(`Version ${data.version} is now available 🎉`))
          .append($('<p>').html(`<a href="${data.release_url}" target="_blank">View release details</a>`)),
        buttons: [
          {
            text: 'Ignore this version',
            class: 'btn-default',
            pullLeft: true,
            onClick: () => localStorage.setItem('pat_ignored_version', data.version)
          },
          {
            text: 'Remind me later',
            class: 'btn-default',
            onClick: () => localStorage.setItem('pat_version_check_time', new Date().getTime())
          },
          {
            text: 'Download',
            class: 'btn-primary',
            onClick: () => window.open(data.release_url, '_blank')
          }
        ]
      });
    },
    error: function(xhr, textStatus, errorThrown) {
      console.log('Version check failed:', textStatus, errorThrown);
    }
  });
}
