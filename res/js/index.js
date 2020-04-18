var wsURL = "";
var posId = 0;
var connectAliases;
var mycall = "";

var statusDiv;
var statusPos = $('#pos_status');

function initFrontend(ws_url)
{
	wsURL = ws_url;

	$( document ).ready(function() {

		initStatusModal();

		// Setup actions
		$('#connect_btn').click(connect);
		$('#connectForm input').keypress(function (e) {
			if (e.which == 13) {
				connect();
				return false;
			}
		});
		$('#connectForm input').keyup(function (e) {
			onConnectInputChange();
		});
		$('#pos_btn').click(postPosition);

		// Setup composer
		initComposeModal();

		// Setup folder navigation
		$('#inbox_tab').click(function(evt){ displayFolder("in") });
		$('#outbox_tab').click(function(evt){ displayFolder("out") });
		$('#sent_tab').click(function(evt){ displayFolder("sent") });
		$('#archive_tab').click(function(evt){ displayFolder("archive") });
		$('.navbar a').click(function(e) {
			var $this = $(this);
			if (!$this.hasClass('dropdown-toggle')) {
				$('.navbar a.active').removeClass('active');
				if (!$this.hasClass('active')) {
					$this.addClass('active');
				}
			}
			e.preventDefault();
		});

		$('#posModal').on('shown.bs.modal', function (e) {
			$.ajax({
				url: '/api/current_gps_position',
				dataType: 'json',
				beforeSend: function(){
					statusPos.html("Checking if GPS device is available");
				},
				success: function(gpsData){
					statusPos.html("GPS position received");

					statusPos.html("<strong>Waiting for position form GPS device...</strong>");
					updatePositionGPS(gpsData);
				},
				error: function( jqXHR, textStatus, errorThrown ){
					statusPos.html("GPS device not available!");

					if (navigator.geolocation) {
						statusPos.html("<strong>Waiting for position (geolocation)...</strong>");
						var options = { enableHighAccuracy: true, maximumAge: 0 };
						posId = navigator.geolocation.watchPosition(updatePositionGeolocation, handleGeolocationError, options);
					} else {
						statusPos.html("Geolocation is not supported by this browser.");
					}
				}
			});
		});

		$('#posModal').on('hidden.bs.modal', function (e) {
			if (navigator.geolocation) {
				navigator.geolocation.clearWatch(posId);
			}
		});

		initConnectModal();

		initConsole();
		displayFolder("in");

		initNotifications();
		initForms();
	});
}

function initNotifications() {
	if( !isNotificationsSupported() ){
		statusDiv.find('#notifications_error').find('.card-text').html('Not supported by this browser.');
		return
	}
	Notification.requestPermission(function(permission) {
		if( permission === "granted" ){
			showGUIStatus(statusDiv.find('#notifications_error'), false)
		} else if (isInsecureOrigin()) {
			// There is no way of knowing for sure if the permission was denied by the user
			// or prohibited because of insecure origin (Chrome). This is just a lucky guess.
			appendInsecureOriginWarning(statusDiv.find('#notifications_error'))
		}
	});
}

function isNotificationsSupported() {
    if( !window.Notification || !Notification.requestPermission )
        return false;

    if( Notification.permission === 'granted' )
	return true;

    // Chrome on Android support notifications only in the context of a Service worker.
    // This is a hack to detect this case, so we can avoid asking for a pointless permission.
    try {
        new Notification('');
    } catch (e) {
        if (e.name == 'TypeError')
            return false;
    }
    return true;
}

var cancelCloseTimer = false
function updateProgress(p) {
	cancelCloseTimer=!p.done

	if( p.receiving || p.sending ){
		percent=Math.ceil(p.bytes_transferred*100 / p.bytes_total);
		op = p.receiving ? "Receiving" : "Sending";
		text = op + " " + p.mid + " (" + p.bytes_total + " bytes)"
		if( p.subject ){
			text += " - " + htmlEscape(p.subject)
		}
		$('#navbar_progress .progress-bar').css("width", percent + "%").text(text + " " + percent + "%");
	}

	if( !$('#navbar_progress').hasClass("d-none") && p.done ){
		window.setTimeout(function() {
			if (!cancelCloseTimer) {
				$('#navbar_progress').addClass("d-none");
			}
		}, 3000);
	} else if( (p.receiving || p.sending) && !p.done ){
		$('#navbar_progress').removeClass("d-none");
	}
}

function initStatusModal() {
	statusDiv = $('#statusModal');
	showGUIStatus($('#websocket_error'), true);
	showGUIStatus($('#notifications_error'), true);

	$('.navbar-brand').click(function(e){ $('#statusModal').modal('toggle'); })
}

function onFormLaunching() {
	$('#selectForm').modal('hide')
	startPollingFormData()
}

function startPollingFormData() {
	setCookie("forminstance", Math.floor(Math.random() * 1000000000), 1);
	pollFormData()
}

function forgetFormData() {
	deleteCookie("forminstance");
}

function pollFormData() {
	$.get(
		'api/form',
		{},
		function(data) {
			console.log(data)
			if ($('#composer').hasClass('show') && (!data.TargetForm || !data.TargetForm.Name)) {
				window.setTimeout(pollFormData, 1000)
			} else {
				console.log("done polling")
				if ($('#composer').hasClass('show') && data.TargetForm && data.TargetForm.Name) {
					writeFormDataToComposer(data)
				}
			}
		},
		'json'
	)
}

function writeFormDataToComposer(data) {
	if (data.TargetForm) {
		$('#msg_body').val(data.MsgBody)
		$('#msg_subject').val(data.MsgSubject)
	}
}

function initComposeModal() {
	$('#compose_btn').click(function(evt){ $('#composer').modal('toggle'); });
	var tokenfieldConfig = {
		delimiter: [',',';',' '], // Must be in sync with SplitFunc (utils.go)
		inputType: 'email',
		createTokensOnBlur: true,
	};
	$('#msg_to').tokenfield(tokenfieldConfig);
	$('#msg_cc').tokenfield(tokenfieldConfig);
	$('#composer').on('change', '.btn-file :file', previewAttachmentFiles);
	$('#composer').on('hide.bs.modal', forgetFormData);
	$('#composer').on('shown.bs.modal', function() {
			$('.formLaunch').click( onFormLaunching );
		}
	);

	$('#composer_error').hide();

	$('#compose_cancel').click(function(evt){
		closeComposer(true);
	});

	$('#composer_form').submit(function(e) {
		var form = $('#composer_form');
		var d = new Date().toJSON();
		$("#msg_form_date").remove();
		form.append('<input id="msg_form_date" type="hidden" name="date" value="' + d + '" />');

		// Set some defaults that makes the message pass validation (as Winlink Express does)
		if( $('#msg_body').val().length == 0 ){
			$('#msg_body').val('<No message body>');
		}
		if( $('#msg_subject').val().length == 0 ){
			$('#msg_subject').val('<No subject>');
		}

		$.ajax({
			url: "/api/mailbox/out",
			method: "POST",
			data: new FormData(form[0]),
			processData: false,
			contentType: false,
			success: function(result) {
				$('#composer').modal('hide');
				closeComposer(true);
				alert(result);
			},
			error: function(error) {
				$('#composer_error').html(error.responseText);
				$('#composer_error').show();
			},
		});
		e.preventDefault();
	});
}

function initForms() {
	$.getJSON("/api/formcatalog", function(data){
		$('#formsVersion').text("(v" + data.Version + ")");
		$('#formsRootFolderName').text(data.Path);
		appendFormFolder('formFolderRoot', data);
	});
}

function setCookie(cname, cvalue, exdays) {
  var d = new Date();
  d.setTime(d.getTime() + (exdays*24*60*60*1000));
  var expires = "expires="+ d.toUTCString();
  document.cookie = cname + "=" + cvalue + ";" + expires + ";path=/";
}

function deleteCookie(cname) {
  document.cookie = cname + "=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
}

function appendFormFolder(rootId, data) {
	if (data.Folders && data.Folders.length > 0) {
		var rootAcc = `${rootId}Acc`
		$(`#${rootId}`).append(`
			<div class="accordion" id="${rootAcc}">
			</div>
			`);
		data.Folders.forEach(function (folder) {
			var folderNameId = folder.Name.replace( /\s/g, "_" );
			var cardBodyId = folderNameId+"Body";
			var card =
			`
			<div class="card">
				<div class="card-header d-flex">
					<button class="btn btn-secondary flex-fill" type="button" data-toggle="collapse" data-target="#${folderNameId}">
						${folder.Name}
					</button>
				</div>
				<div id="${folderNameId}" class="collapse" data-parent="#${rootAcc}">
					<div class="card-body" id=${cardBodyId}>
					</div>
				</div>
			</div>
			`
			$(`#${rootAcc}`).append(card)
			appendFormFolder(`${cardBodyId}`, folder)
			if (folder.Forms && folder.Forms.length > 0){
				var cardBodyFormsId = `${cardBodyId}Forms`
				$(`#${cardBodyId}`).append( `<div id="${cardBodyFormsId}" class="list-group"></div>` )
				folder.Forms.forEach((form) => {
					var pathEncoded = encodeURIComponent(form.InitialURI)
					$(`#${cardBodyFormsId}`).append(`<a href="api/forms?formPath=${pathEncoded}" target="_blank" class="list-group-item list-group-item-action list-group-item-light formLaunch">${form.Name}</a>`)
				});
			}
		});
	}
}

function initConnectModal() {
	$('#freqInput').change(onConnectInputChange);
	$('#radioOnlyInput').change(onConnectInputChange);
	$('#addrInput').change(onConnectInputChange);
	$('#targetInput').change(onConnectInputChange);

	$('#transportSelect').change(function() {
		refreshExtraInputGroups();
		onConnectInputChange();
	});
	refreshExtraInputGroups();

	updateConnectAliases();
}

function updateConnectAliases() {
	$.getJSON("/api/connect_aliases", function(data){
		connectAliases = data;

		var select = $('#aliasSelect');
		Object.keys(data).forEach(function (key) {
			select.append(new Option(key));
		});

		select.change(function() {
			$('#aliasSelect option:selected').each(function() {
				var alias = $(this).text();
				var url = connectAliases[$(this).text()];
				setConnectValues(url);
				select.val('');
				select.selectpicker('refresh');
			});
		});
		select.selectpicker('refresh');
	});
}

function setConnectValues(url) {
	url=URI(url.toString());

	$('#transportSelect').val(url.protocol());
	$('#transportSelect').selectpicker('refresh');
	refreshExtraInputGroups();

	$('#targetInput').val(url.path().substr(1));

	var query = url.search(true);

	if(url.hasQuery("freq")) {
		$('#freqInput').val(query["freq"])
	} else {
		$('#freqInput').val('');
	}

	if(url.hasQuery("radio_only")) {
		$('#radioOnlyInput')[0].checked = query["radio_only"];
	} else {
		$('#radioOnlyInput')[0].checked = false
	}

	var usri = ""
	if(url.username()) {
		usri += url.username()
	}
	if(url.password()) {
		usri += ":" + url.password()
	}
	if(usri != "") {
		usri += "@"
	}
	$('#addrInput').val(usri + url.host());

	onConnectInputChange();
}

function getConnectURL() {
	var url = $('#transportSelect').val() + "://" + $('#addrInput').val() + "/" + $('#targetInput').val();

	params = "";

	if($('#freqInput').val()) {
		params += "&freq=" + $('#freqInput').val();
	}
	if($('#radioOnlyInput').is(':checked')) {
		params += "&radio_only=true";
	}

	if(params) {
		url += params.replace("&", "?");
	}

	return url;
}

function onConnectInputChange() {
	$('#connectURLPreview').empty().append(getConnectURL());
}

function refreshExtraInputGroups() {
	var transport = $('#transportSelect').val();
	if(transport == "telnet") {
		$('#freqInputDiv').hide();
		$('#freqInput').val('');
		$('#addrInputDiv').show();
	} else {
		$('#addrInputDiv').hide();
		$('#addrInput').val('');
		$('#freqInputDiv').show();
	}

	if(transport == "ax25" || transport == "serial-tnc") {
		$('#radioOnlyInput')[0].checked = false;
		$('#radioOnlyInputDiv').hide();
	} else {
		$('#radioOnlyInputDiv').show();
	}
}

function handleGeolocationError(error) {
	if(error.message.search("insecure origin") > 0 || isInsecureOrigin()) {
		appendInsecureOriginWarning(statusDiv.find('#geolocation_error'))
	}
	showGUIStatus(statusDiv.find('#geolocation_error'), true)
	statusPos.html("Geolocation unavailable.");
}

function updatePositionGeolocation(pos) {
	var d = new Date(pos.timestamp);
	statusPos.html("Last position update " + dateFormat(d) + "...");
	$('#pos_lat').val(pos.coords.latitude);
	$('#pos_long').val(pos.coords.longitude);
	$('#pos_ts').val(pos.timestamp);
}

function updatePositionGPS(pos) {
	var d = new Date(pos.Time);
	statusPos.html("Last position update " + dateFormat(d) + "...");
	$('#pos_lat').val(pos.Lat);
	$('#pos_long').val(pos.Lon);
	$('#pos_ts').val(d.getTime());
}

function postPosition() {
	var pos = {
		lat:  parseFloat($('#pos_lat').val()),
		lon: parseFloat($('#pos_long').val()),
		comment:   $('#pos_comment').val(),
		date: new Date(parseInt($('#pos_ts').val())),
	};

	$.ajax("/api/posreport", {
		data : JSON.stringify(pos),
		contentType : 'application/json',
		type : 'POST',
		success: function(resp) {
			$('#posModal').modal('toggle');
			alert(resp);
		},
		error: function(xhr, st, resp) {
			alert(resp + ": " + xhr.responseText);
		},
	});
}

function previewAttachmentFiles() {
	var files = $(this).get(0).files;
	attachments = $('#composer_attachments');
	attachments.empty();
	for (var i = 0; i < files.length; i++) {
		file = files.item(i);

		if(isImageSuffix(file.name)){
			var reader = new FileReader();
			reader.onload = function(e) {
				attachments.append(
					'<div class="col-xs-6 col-md-3"><a href="#" class="btn btn-light btn-sm"><span class="fas fa-paperclip"></span> ' +
					(file.size/1024).toFixed(2) + 'kB' +
					'<img class="img-fluid img-thumbnail" src="'+ e.target.result + '" alt="' + file.name + '">' +
					'</a></div>'
				);
			}
			reader.readAsDataURL(file);
		} else {
			attachments.append(
				'<div class="col-xs-6 col-md-3"><a href="#" class="btn btn-light btn-sm"><span class="fas fa-paperclip"></span> ' +
				file.name + '<br />(' + (file.size/1024).toFixed(2) + 'kB)' +
				'</a></div>'
			);
		}
	};
}

function notify(data)
{
	var options = {
		body: data.body,
		icon: '/res/images/pat_logo.png',
	}
	var n = new Notification(data.title, options);
}

function alert(msg)
{
	var div = $('#navbar_status');
	div.empty();
	div.append('<span class="navbar-text status-text">' + msg + '</span>');
	div.show();
	window.setTimeout(function() { div.fadeOut(500); }, 5000);
}

function updateStatus(data)
{
	var st = $('#status_text');
	st.empty();

	if(data.connected){
		st.append("Connected " + data.remote_addr + "");
	} else if(data.active_listeners.length > 0){
		st.append("<i>Listening " + data.active_listeners + "</i>");
	} else {
		st.append("<i>Idle</i>");
	}

	var n = data.http_clients.length;
	statusDiv.find('#webserver_info').find('.card-text').html(n + (n == 1 ? ' client ' : ' clients ') + 'connected.');
}

function closeComposer(clear)
{
	if(clear){
		$('#composer_error').val('').hide();
		$('#msg_body').val('')
		$('#msg_subject').val('')
		$('#msg_to').tokenfield('setTokens', '')
		$('#msg_cc').tokenfield('setTokens', '')
		$('#composer_form')[0].reset();

		// Attachment previews
		$('#composer_attachments').empty();

		// Attachment input field
		var attachments = $('#msg_attachments_input');
		attachments.replaceWith(attachments = attachments.clone(true));
	}
	$('#composer').modal('hide');
}

function connect(evt)
{
	url = getConnectURL()
	$('#connectModal').modal('hide');

	$.getJSON("/api/connect?url=" + url, function(data){
		if( data.NumReceived == 0 ){
			window.setTimeout(function() { alert("No new messages."); }, 1000);
		}
	}).fail(function() {
		alert("Connect failed. See console for detailed information.");
	});
}

function updateGUIStatus()
{
	var color = "success";
	statusDiv.find('.bg-info').not('.d-none').not('.ignore-status').each(function(i) {
		color = "info";
	});
	statusDiv.find('.bg-warning').not('.d-none').not('.ignore-status').each(function(i) {
		color = "warning";
	});
	statusDiv.find('.bg-danger').not('.d-none').not('.ignore-status').each(function(i) {
		color = "danger";
	});
	$('#gui_status_light').removeClass (function (index, className) {
		return (className.match (/(^|\s)btn-\S+/g) || []).join(' ');
	}).addClass('btn-' + color);
	if(color == "success") {
		statusDiv.find('#no_error').removeClass('d-none');
	} else {
		statusDiv.find('#no_error').addClass('d-none');
	}
}

function isInsecureOrigin()
{
	if(hasOwnProperty.call(window, 'isSecureContext')){ return !window.isSecureContext }
	if(window.location.protocol == 'https:'){ return false }
	if(window.location.protocol == 'file:'){ return false }
	if(location.hostname === "localhost" || location.hostname.startsWith("127.0")){ return false }
	return true
}

function appendInsecureOriginWarning(e)
{
	e.removeClass('bg-info').addClass('bg-warning')
	e.find('.card-text').append('<p>Ensure the <a href="https://github.com/la5nta/pat/wiki/The-web-GUI#powerful-features">secure origin criteria for Powerful Features</a> are met.</p>')
	updateGUIStatus()
}

function showGUIStatus(e, show)
{
	show ? e.removeClass('d-none') : e.addClass('d-none');
	updateGUIStatus();
}

var ws;

function initConsole()
{
	if("WebSocket" in window){
		ws = new WebSocket(wsURL);
		ws.onopen    = function(evt) {
			showGUIStatus(statusDiv.find('#websocket_error'), false);
			showGUIStatus(statusDiv.find('#webserver_info'), true);
			$('#console').empty();
		};
		ws.onmessage = function(evt) {
			var msg = JSON.parse(evt.data);
			if(msg.MyCall) {
				mycall = msg.MyCall
			}
			if(msg.Notification) {
				notify(msg.Notification)
			}
			if(msg.LogLine) {
				updateConsole(msg.LogLine + "\n");
			}
			if(msg.UpdateMailbox) {
				displayFolder(currentFolder);
			}
			if(msg.Status) {
				updateStatus(msg.Status)
			}
			if(msg.Progress) {
				updateProgress(msg.Progress)
			}
			if(msg.Prompt) {
				processPromptQuery(msg.Prompt)
			}
			if(msg.PromptAbort) {
				$('#promptModal').modal('hide');
			}
		};
		ws.onclose   = function(evt) {
			showGUIStatus(statusDiv.find('#websocket_error'), true)
			showGUIStatus(statusDiv.find('#webserver_info'), false)
			window.setTimeout(function() { initConsole(); }, 1000);
		};
	} else {
		// The browser doesn't support WebSocket
		wsError = true;
		alert("Websocket not supported by your browser, please upgrade your browser.");
	}
}

function processPromptQuery(p)
{
	console.log(p)

	if(p.kind != "password"){
		console.log("Ignoring unsupported prompt of kind: " + p.kind)
		return;
	}

	$('#promptID').val(p.id);
	$('#promptResponseValue').val('');
	$('#promptMessage').text(p.message)
	$('#promptModal').modal('show');
}

function postPromptResponse()
{
	var id = $('#promptID').val();
	var value = $('#promptResponseValue').val();
	$('#promptModal').modal('hide');
	ws.send(JSON.stringify({
		prompt_response: {
			id:    id,
			value: value,
		},
	}));
}

function updateConsole(msg)
{
	var pre = $('#console')
	pre.append('<span class="terminal">' + msg + '</span>');
	pre.scrollTop( pre.prop("scrollHeight") );
}

const getCellValue = (tr, idx) => tr.children[idx].innerText || tr.children[idx].textContent;

const comparer = (idx, asc) => (a, b) => ((v1, v2) =>
		v1 !== '' && v2 !== '' && !isNaN(v1) && !isNaN(v2) ? v1 - v2 : v1.toString().localeCompare(v2)
)(getCellValue(asc ? a : b, idx), getCellValue(asc ? b : a, idx));

var currentFolder;
function displayFolder(dir) {
	currentFolder = dir;

	var is_from = (dir == "in" || dir == "archive");

	var table = $('#folder table');
	table.empty();
	table.append(
		  "<thead><tr><th></th><th>Subject</th>"
		+ "<th>" + (is_from ? "From" : "To") + "</th>"
		+ (is_from ? "" : "<th>P2P</th>")
		+ "<th>Date</th><th>Message ID</th></tr></thead><tbody></tbody>");

	var tbody = $('#folder table tbody');

	$.getJSON("/api/mailbox/" + dir, function(data){
		for(var i = 0; i < data.length; i++){
			var msg = data[i];

			//TODO: Cleanup (Sorry about this...)
			var html = '<tr id="' + msg.MID + '" class="active' + (msg.Unread ? ' strong' : '') + '"><td>';
			if(msg.Files.length > 0){
				html += '<span class="glyphicon glyphicon-paperclip"></span>';
			}
			html += '</td><td>' + htmlEscape(msg.Subject) + "</td><td>";
			if( !is_from && !msg.To ){
				html += '';
			} else if( is_from ) {
				html += msg.From.Addr;
			} else if( msg.To.length == 1 ){
				html += msg.To[0].Addr;
			} else if( msg.To.length > 1 ){
				html += msg.To[0].Addr + "...";
			}
			html += '</td>'
			html += (is_from ? '' : '<td>' + (msg.P2POnly ? '<span class="glyphicon glyphicon-ok"></span>' : '') + '</td>')
			html += '<td>' + msg.Date + '</td><td>' + msg.MID + '</td></tr>';

			var elem = $(html)
			tbody.append(elem);
			elem.click(function(evt){
				displayMessage($(this));
			});
		}
	});
	// Adapted from https://stackoverflow.com/a/49041392
	document.querySelectorAll('th').forEach(th => th.addEventListener('click', (() => {
		const table = th.closest('table');
		const tbody = table.querySelector('tbody');
		Array.from(tbody.querySelectorAll('tr'))
			.sort(comparer(Array.from(th.parentNode.children).indexOf(th), this.asc = !this.asc))
			.forEach(tr => tbody.appendChild(tr));
		const previousTh = table.querySelector('th.sorted');
		if (previousTh != null) {
			previousTh.classList.remove('sorted');
		}
		th.classList.add('sorted');
	})));
}

function displayMessage(elem) {
	var mid = elem.attr('ID');
	var msg_url = "/api/mailbox/" + currentFolder + "/" + mid;

	$.getJSON(msg_url, function(data){
		elem.attr('class', 'info');

		var view = $('#message_view');
		view.find('#subject').text(data.Subject);
		view.find('#headers').empty();
		view.find('#headers').append('Date: ' + data.Date + '<br />');
		view.find('#headers').append('From: ' + data.From.Addr + '<br />');
		view.find('#headers').append('To: ');
		for(var i = 0; data.To && i < data.To.length; i++){
			view.find('#headers').append('<el>' + data.To[i].Addr + '</el>' + (data.To.length-1 > i ? ', ' : ''));
		}
		if(data.P2POnly){
			view.find('#headers').append(' (<strong>P2P only</strong>)');
		}

		if(data.Cc){
			view.find('#headers').append('<br />Cc: ');
			for(var i = 0; i < data.Cc.length; i++){
				view.find('#headers').append('<el>' + data.Cc[i].Addr + '</el>' + (data.Cc.length-1 > i ? ', ' : ''));
			}
		}

		view.find('#body').html(data.BodyHTML);

		var attachments = view.find('#attachments');
		attachments.empty();
		if(!data.Files){
			attachments.hide();
		} else {
			attachments.show();
		}
		for(var i = 0; data.Files && i < data.Files.length; i++){
			var file = data.Files[i];

			if(isImageSuffix(file.Name)) {
				attachments.append(
					'<div class="col-xs-6 col-md-3"><a target="_blank" href="' + msg_url + "/" + file.Name + '" class="btn btn-light btn-sm"><span class="fas fa-paperclip"></span> ' +
					(file.Size/1024).toFixed(2) + 'kB' +
					'<img class="img-fluid img-thumbnail" src="' + msg_url + "/" + file.Name + '" alt="' + file.Name + '">' +
					'</a></div>'
				);
			} else {
				attachments.append(
					'<div class="col-xs-6 col-md-3"><a target="_blank" href="' + msg_url + "/" + file.Name + '" class="btn btn-light btn-sm"><span class="fas fa-paperclip"></span> ' +
					file.Name + '<br />(' + (file.Size/1024).toFixed(2) + 'kB)' +
					'</a></div>'
				);
			}
		}
		$('#reply_btn').off('click');
		$('#reply_btn').click(function(evt){
			$('#message_view').modal('hide');

			$('#msg_to').tokenfield('setTokens', [data.From.Addr]);
			$('#msg_cc').tokenfield('setTokens', replyCarbonCopyList(data));
			if(data.Subject.lastIndexOf("Re:", 0) != 0) {
				$('#msg_subject').val("Re: " +  data.Subject);
			} else {
				$('#msg_subject').val(data.Subject);
			}
			$('#msg_body').val(quoteMsg(data));

			$('#composer').modal('show');
		});
		$('#forward_btn').off('click');
		$('#forward_btn').click(function(evt){
			$('#message_view').modal('hide');

			$('#msg_subject').val("Fw: " +  data.Subject);
			$('#msg_body').val(quoteMsg(data));

			$('#composer').modal('show');
		});
		$('#delete_btn').off('click');
		$('#delete_btn').click(function(evt){
			deleteMessage(currentFolder, mid);
		});
		$('#archive_btn').off('click');
		$('#archive_btn').click(function(evt){
			archiveMessage(currentFolder, mid);
		});

		// Archive button should be hidden for already archived messages
		if( currentFolder == "archive" ){
			$('#archive_btn').parent().hide();
		} else {
			$('#archive_btn').parent().show();
		}

		view.show();
		$('#message_view').modal('show');
		mbox = currentFolder
		if(!data.Read) {
			window.setTimeout(function() { setRead(mbox, data.MID); }, 2000);
		}
		elem.attr('class', 'active');
	});
}

function replyCarbonCopyList(msg) {
	var addrs = msg.To
	if(msg.Cc != null && msg.Cc.length > 0){
		addrs = addrs.concat(msg.Cc)
	}
	var seen = {}; seen[mycall] = true; seen[msg.From.Addr] = true;
	var strings = [];
	for(var i = 0; i < addrs.length; i++){
		if(seen[addrs[i].Addr]){
			continue
		}
		seen[addrs[i].Addr] = true
		strings.push(addrs[i].Addr);
	}
	return strings;
}

function quoteMsg(data) {
	var output =  "--- " + data.Date + " " + data.From.Addr + " wrote: ---\n";

	var lines = data.Body.split('\n')
	for(var i = 0;i < lines.length;i++){
		output += ">" + lines[i] + "\n"
	}
	return output
}

function htmlEscape(str) {
	return $('<div/>').text(str).html();
}

function archiveMessage(box, mid) {
	$.ajax("/api/mailbox/archive", {
		headers: {
			"X-Pat-SourcePath": "/api/mailbox/"+box+"/"+mid,
		},
		contentType : 'application/json',
		type : 'POST',
		success: function(resp) {
			$('#message_view').modal('hide');
			alert("Message archived");
		},
		error: function(xhr, st, resp) {
			alert(resp + ": " + xhr.responseText);
		},
	});
}

function deleteMessage(box, mid) {
	$('#confirm_delete').on('click', '.btn-ok', function(e) {
		$('#message_view').modal('hide');
		var $modalDiv = $(e.delegateTarget);
		$.ajax("/api/mailbox/"+box+"/"+mid, {
			type : 'DELETE',
			success: function(resp) {
				$modalDiv.modal('hide');
				alert("Message deleted");
			},
			error: function(xhr, st, resp) {
				$modalDiv.modal('hide');
				alert(resp + ": " + xhr.responseText);
			},
		});
	});
	$('#confirm_delete').modal('show');
}

function setRead(box, mid) {
	var data = {read: true};

    $.ajax("/api/mailbox/" + box + "/" + mid + "/read", {
		data : JSON.stringify(data),
		contentType : 'application/json',
		type : 'POST',
		success: function(resp) {},
        error: function(xhr, st, resp) {
            alert(resp + ": " + xhr.responseText);
        },
    });
}

function isImageSuffix(name) {
	return name.toLowerCase().match(/\.(jpg|jpeg|png|gif)$/);
}

function dateFormat(previous) {
	var current = new Date();

    var msPerMinute = 60 * 1000;
    var msPerHour = msPerMinute * 60;
    var msPerDay = msPerHour * 24;
    var msPerMonth = msPerDay * 30;
    var msPerYear = msPerDay * 365;

    var elapsed = current - previous;

    if (elapsed < msPerDay ) {
		return (previous.getHours() < 10 ?"0":"") + previous.getHours() + ':' + (previous.getMinutes() < 10 ?"0":"") + previous.getMinutes();
    } else if (elapsed < msPerMonth) {
        return 'approximately ' + Math.round(elapsed/msPerDay) + ' days ago';
    } else if (elapsed < msPerYear) {
        return 'approximately ' + Math.round(elapsed/msPerMonth) + ' months ago';
    } else {
        return 'approximately ' + Math.round(elapsed/msPerYear ) + ' years ago';
    }
}
