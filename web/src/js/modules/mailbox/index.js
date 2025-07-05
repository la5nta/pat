import $ from 'jquery';
import { htmlEscape } from '../utils/index.js';

export class Mailbox {
  constructor(onMessageClick) {
    this.onMessageClick = onMessageClick;
    this.currentFolder = 'in';
  }

  init() {
    // Setup folder navigation
    $('#inbox_tab').click(() => this.displayFolder('in'));
    $('#outbox_tab').click(() => this.displayFolder('out'));
    $('#sent_tab').click(() => this.displayFolder('sent'));
    $('#archive_tab').click(() => this.displayFolder('archive'));

    $('#inbox_tab, #outbox_tab, #sent_tab, #archive_tab')
      .parent('li')
      .click(function(e) {
        $('.navbar li.active').removeClass('active');
        const $this = $(this);
        if (!$this.hasClass('active')) {
          $this.addClass('active');
        }
        e.preventDefault();
      });
  }

  displayFolder(dir) {
    this.currentFolder = dir;
    const is_from = dir === 'in' || dir === 'archive';

    const table = $('#folder table');
    table.empty();
    table.append(
      `<thead><tr><th></th><th>Subject</th>
      <th>${is_from ? 'From' : 'To'}</th>
      ${is_from ? '' : '<th>P2P</th>'}
      <th>Date</th><th>Message ID</th></tr></thead><tbody></tbody>`
    );

    const tbody = $('#folder table tbody');

    $.getJSON(`/api/mailbox/${dir}`, (data) => {
      data.forEach((msg) => {
        let to_from_html = '';
        if (!is_from && msg.To) {
          if (msg.To.length === 1) {
            to_from_html = msg.To[0].Addr;
          } else if (msg.To.length > 1) {
            to_from_html = `${msg.To[0].Addr}...`;
          }
        } else if (is_from) {
          to_from_html = msg.From.Addr;
        }

        const p2p_html = is_from
          ? ''
          : `<td>${msg.P2POnly ? '<span class="glyphicon glyphicon-ok"></span>' : ''}</td>`;

        const elem = $(`
          <tr id="${msg.MID}" class="active${msg.Unread ? ' strong' : ''}">
            <td>${msg.Files.length > 0 ? '<span class="glyphicon glyphicon-paperclip"></span>' : ''}</td>
            <td>${htmlEscape(msg.Subject)}</td>
            <td>${to_from_html}</td>
            ${p2p_html}
            <td>${msg.Date}</td>
            <td>${msg.MID}</td>
          </tr>
        `);

        tbody.append(elem);
        elem.click((evt) => {
          // Handle active class for the message list
          tbody.find('tr.active').removeClass('active');
          elem.addClass('active');

          this.onMessageClick(this.currentFolder, elem.attr('id'));
        });
      });
    });

    // Adapted from https://stackoverflow.com/a/49041392
    document.querySelectorAll('th').forEach((th) => {
      th.addEventListener('click', (event) => {
        const clickedTh = event.currentTarget;
        const table = clickedTh.closest('table');
        const tbody = table.querySelector('tbody');
        Array.from(tbody.querySelectorAll('tr'))
          .sort(this._comparer(Array.from(clickedTh.parentNode.children).indexOf(clickedTh), (clickedTh.asc = !clickedTh.asc)))
          .forEach((tr) => tbody.appendChild(tr));
        const previousTh = table.querySelector('th.sorted');
        if (previousTh && previousTh !== clickedTh) {
          previousTh.classList.remove('sorted');
        }
        clickedTh.classList.add('sorted');
      });
    });
  }

  _getCellValue(tr, idx) {
    return tr.children[idx].innerText || tr.children[idx].textContent;
  }

  _comparer(idx, asc) {
    return (a, b) =>
      ((v1, v2) =>
        v1 !== '' && v2 !== '' && !isNaN(v1) && !isNaN(v2)
          ? v1 - v2
          : v1.toString().localeCompare(v2))(
            this._getCellValue(asc ? a : b, idx),
            this._getCellValue(asc ? b : a, idx)
          );
  }
}
