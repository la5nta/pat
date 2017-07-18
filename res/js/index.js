var wsURL = "";
var wsError = false;
var posId = 0;
var connectAliases;

var uploadFiles = new Array();

function initFrontend(ws_url)
{
	wsURL = ws_url;

	$( document ).ready(function() {
		//$('select').selectpicker();

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

		$('#compose_btn').click(function(evt){ $('#composer').modal('toggle'); });
		$('#pos_btn').click(postPosition);
		
		// Setup folder navigation
		$('#inbox_tab').click(function(evt){ displayFolder("in") });
		$('#outbox_tab').click(function(evt){ displayFolder("out") });
		$('#sent_tab').click(function(evt){ displayFolder("sent") });
		$('#archive_tab').click(function(evt){ displayFolder("archive") });
		$('.navbar li').click(function(e) {
			$('.navbar li.active').removeClass('active');
			var $this = $(this);
			if (!$this.hasClass('active')) {
				$this.addClass('active');
			}
			e.preventDefault();
		});

		$('.nav :not(.dropdown) a').on('click', function(){
    		if($('.navbar-toggle').css('display') !='none'){
				$(".navbar-toggle").trigger( "click" );
			}
		});

		$('#composer').on('change', '.btn-file :file', previewAttachmentFiles);

		$('#post_btn').click(postMessage);

		$('#posModal').on('shown.bs.modal', function (e) {
			if (navigator.geolocation) {
				$('#pos_status').empty().append("<strong>Waiting for position...</strong>");
				posId = navigator.geolocation.watchPosition(updatePosition);
			} else { 
				$('#pos_status').empty().append("Geolocation is not supported by this browser.");
			}
		});
		$('#posModal').on('hidden.bs.modal', function (e) {
			if (navigator.geolocation) {
				navigator.geolocation.clearWatch(posId);
			}
		});

		setupConnectModal();

		initConsole();
		displayFolder("in");
	});
}

var cancelCloseTimer = false
function updateProgress(p) {
	cancelCloseTimer=!p.done

	if( p.receiving || p.sending ){
		percent=Math.ceil(p.bytes_transferred*100 / p.bytes_total);
		op = p.receiving ? "Receiving" : "Sending";
		text = op + " " + p.mid + " (" + p.bytes_total + " bytes)"
		if( p.subject ){
			text += " - " + p.subject
		}
		$('#navbar_progress .progress-text').text(text);
		$('#navbar_progress .progress-bar').css("width", percent + "%").text(percent + "%");
	}

	if( $('#navbar_progress').is(':visible') && p.done ){
		window.setTimeout(function() {
			if (!cancelCloseTimer) {
				$('#navbar_progress').fadeOut(500);
			}
		}, 3000);
	} else if( (p.receiving || p.sending) && !p.done ){
		$('#navbar_progress').show();
	}
}

function setupConnectModal() {
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
			select.append('<option>' + key + '</option>');
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

function updatePosition(pos) {
	var d = new Date(pos.timestamp);
	$('#pos_status').empty().append("Last position update " + dateFormat(d) + "...");
	$('#pos_lat').val(pos.coords.latitude);
	$('#pos_long').val(pos.coords.longitude);
	$('#pos_ts').val(pos.timestamp);
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

function postMessage() {
	var iframe = $('<iframe name="postiframe" id="postiframe" style="display: none"></iframe>');

	$("body").append(iframe);

	var form = $('#composer_form');
	form.attr("action", "/api/mailbox/out");
	form.attr("method", "POST");

	form.attr("encoding", "multipart/form-data");
	form.attr("enctype", "multipart/form-data");

	form.attr("target", "postiframe");

	// Use client's date
	var d = new Date().toJSON();
	$("#msg_form_date").remove();
	form.append('<input id="msg_form_date" type="hidden" name="date" value="' + d + '" />');

	form.submit();

	$("#postiframe").load(function () {
		iframeContents = this.contentWindow.document.body.innerHTML;
		$('#composer').modal('hide');
		closeComposer(true);
		alert(iframeContents);
	});

	return false;
}

function previewAttachmentFiles() {
	var files = $(this).get(0).files;
	attachments = $('#composer_attachments');
	for (var i = 0; i < files.length; i++) {
		file = files.item(i);

		uploadFiles[uploadFiles.length] = file;

		if(isImageSuffix(file.name)){
			var reader = new FileReader();
			reader.onload = function(e) {
				attachments.append(
					'<div class="col-xs-6 col-md-3"><a class="thumbnail" href="#" class="btn btn-default navbar-btn"><span class="glyphicon glyphicon-paperclip" /> ' +
					'<img src="'+ e.target.result + '" alt="' + file.name + '">' +
					'</a></div>'
				);
			}
			reader.readAsDataURL(file);
		} else {
			attachments.append(
				'<div class="col-xs-6 col-md-3"><a href="#" class="btn btn-default navbar-btn"><span class="glyphicon glyphicon-paperclip" /> ' +
				file.name + '<br />(' + file.size + ' bytes)' +
				'</a></div>'
			);
		}
	};
}

function alert(msg)
{
    var div = $('#navbar_status');
    div.empty();    
    div.append('<span class="navbar-text status-text">' + msg + '</p>');
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
	}
}

function closeComposer(clear)
{
	if(clear){
		$('#msg_body').val('')
		$('#msg_subject').val('')
		$('#msg_to').val('')
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
	}).error(function() {
		alert("Connect failed. See console for detailed information.");
	});
}

function initConsole()
{
	if("WebSocket" in window){
		var ws = new WebSocket(wsURL);
		ws.onopen    = function(evt) { wsError = false; $('#console').empty(); };
		ws.onmessage = function(evt) {
			var msg = JSON.parse(evt.data);
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
		};
		ws.onclose   = function(evt) {
			wsError = true;
			window.setTimeout(function() { initConsole(); }, 1000);
		};
	} else {
		// The browser doesn't support WebSocket
		wsError = true;
		alert("Websocket not supported by your browser, please upgrade your browser.");
	}
}

function updateConsole(msg)
{
	var pre = $('#console')
	pre.append('<span class="terminal">' + msg + '</span>');
	pre.scrollTop( pre.prop("scrollHeight") );
}

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
				html += '<span class="glyphicon glyphicon-paperclip" />';
			}

			html += '</td><td>' + msg.Subject + "</td>"
			     + '<td>' + (is_from ? msg.From.Addr : msg.To[0].Addr) + (!is_from && msg.To.length > 1 ? "..." : "") + '</td>'
				+ (is_from ? '' : '<td>' + (msg.P2POnly ? '<span class="glyphicon glyphicon-ok" />' : '') + '</td>')
			    + '<td>' + msg.Date + '</td><td>' + msg.MID + '</td></tr>';

			var elem = $(html)
			tbody.append(elem);
			elem.click(function(evt){
				displayMessage($(this));
			});
		}
	});
}

function displayMessage(elem) {
	var mid = elem.attr('ID');
	var msg_url = "/api/mailbox/" + currentFolder + "/" + mid;

	$.getJSON(msg_url, function(data){
		elem.attr('class', 'info');

		var view = $('#message_view');
		view.find('#subject').html(data.Subject);
		view.find('#headers').empty();
		view.find('#headers').append('Date: ' + data.Date + '<br />');
		view.find('#headers').append('From: ' + data.From.Addr + '<br />');
		view.find('#headers').append('To: ');
		for(var i = 0; i < data.To.length; i++){
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
					'<div class="col-xs-6 col-md-3"><a class="thumbnail" target="_blank" href="' + msg_url + "/" + file.Name + '" class="btn btn-default navbar-btn"><span class="glyphicon glyphicon-paperclip" /> ' + (file.Size/1024).toFixed(2) + 'kB' +
					'<img src="' + msg_url + "/" + file.Name + '" alt="' + file.Name + '">' +
					'</a></div>'
				);
			} else {
				attachments.append(
					'<div class="col-xs-6 col-md-3"><a target="_blank" href="' + msg_url + "/" + file.Name + '" class="btn btn-default navbar-btn"><span class="glyphicon glyphicon-paperclip" /> ' +
					file.Name + '<br />(' + file.Size + ' bytes)' +
					'</a></div>'
				);
			}
		}
		$('#reply_btn').off('click');
		$('#reply_btn').click(function(evt){
			$('#message_view').modal('hide');

			$('#msg_to').val(data.From.Addr);
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

function quoteMsg(data) {
	var output =  "--- " + data.Date + " " + data.From.Addr + " wrote: ---\n";

	var lines = data.Body.split('\n')
	for(var i = 0;i < lines.length;i++){
		output += ">" + lines[i] + "\n"
	}
	return output
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
