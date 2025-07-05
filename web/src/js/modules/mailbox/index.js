import $ from 'jquery';
import { htmlEscape } from '../utils/index.js';

export class Mailbox {
  constructor(onMessageClick) {
    this.onMessageClick = onMessageClick;
    this.currentFolder = 'in';
  }

  init() {
    this.folder = $('#folder');
    this.table = this.folder.find('table');

    // Adapted from https://stackoverflow.com/a/49041392
    this.table.on('click', 'th', (event) => {
      const clickedTh = event.currentTarget;
      const tbody = this.table.find('tbody')[0];
      Array.from(tbody.querySelectorAll('tr'))
        .sort(this._comparer(Array.from(clickedTh.parentNode.children).indexOf(clickedTh), (clickedTh.asc = !clickedTh.asc)))
        .forEach((tr) => tbody.appendChild(tr));
      const previousTh = this.table[0].querySelector('th.sorted');
      if (previousTh && previousTh !== clickedTh) {
        previousTh.classList.remove('sorted');
      }
      clickedTh.classList.add('sorted');
    });
  }

  displayFolder(dir) {
    this.currentFolder = dir;
    const is_from = dir === 'in' || dir === 'archive';

    this.table.empty();
    this.table.append(
      `<thead><tr><th></th><th>Subject</th>
      <th>${is_from ? 'From' : 'To'}</th>
      ${is_from ? '' : '<th>P2P</th>'}
      <th>Date</th><th>Message ID</th></tr></thead><tbody></tbody>`
    );

    const tbody = this.table.find('tbody');

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
