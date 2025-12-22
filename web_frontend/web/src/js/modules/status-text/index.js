class StatusText {
  constructor(onReadyClick) {
    this.onReadyClick = onReadyClick;
    this.statusText = null;
  }

  init() {
    this.statusText = $('#status_text');
  }

  update(data) {
    const st = this.statusText;
    st.empty().off('click').attr('data-toggle', 'tooltip').attr('data-placement', 'bottom').tooltip();

    const onDisconnect = () => {
      st.tooltip('hide');
      this.disconnect(false, () => {
        // This will be reset by the next updateStatus when the session is aborted
        st.empty().append('Disconnecting... ');
        // Issue dirty disconnect on second click
        st.off('click').click(() => {
          st.off('click');
          this.disconnect(true);
          st.tooltip('hide');
        });
        st.attr('title', 'Click to force disconnect').tooltip('fixTitle').tooltip('show');
      });
    };

    if (data.dialing) {
      st.append('Dialing... ');
      st.click(onDisconnect);
      st.attr('title', 'Click to abort').tooltip('fixTitle').tooltip('show');
    } else if (data.connected) {
      st.append('Connected ' + data.remote_addr);
      st.click(onDisconnect);
      st.attr('title', 'Click to disconnect').tooltip('fixTitle').tooltip('hide');
    } else {
      if (data.active_listeners.length > 0) {
        st.append('<i>Listening ' + data.active_listeners + '</i>');
      } else {
        st.append('<i>Ready</i>');
      }
      st.attr('title', 'Click to connect').tooltip('fixTitle').tooltip('hide');
      st.click(this.onReadyClick);
    }
  }

  disconnect(dirty, successHandler) {
    if (successHandler === undefined) {
      successHandler = () => { };
    }
    $.post(
      '/api/disconnect?dirty=' + dirty,
      {},
      function(response) {
        successHandler();
      },
      'json'
    );
  }
}

export { StatusText };
