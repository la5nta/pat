import $ from 'jquery';

// Requests against a single message under /api/mailbox, shared by the mailbox
// list, the viewer and the composer so the path and header formats live in
// one place. Each call returns the jqXHR for the caller to chain on.

export function messagePath(folder, mid) {
  return '/api/mailbox/' + encodeURIComponent(folder) + '/' + encodeURIComponent(mid);
}

export function setRead(folder, mid, read) {
  return $.ajax(messagePath(folder, mid) + '/read', {
    data: JSON.stringify({ read: read }),
    contentType: 'application/json',
    type: 'POST',
  });
}

export function moveMessage(folder, mid, target) {
  return $.ajax('/api/mailbox/' + encodeURIComponent(target), {
    headers: { 'X-Pat-SourcePath': messagePath(folder, mid) },
    contentType: 'application/json',
    type: 'POST',
  });
}

export function deleteMessage(folder, mid) {
  return $.ajax(messagePath(folder, mid), { type: 'DELETE' });
}
