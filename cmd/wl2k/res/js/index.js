var wsURL = "";
var wsError = false;
var posId = 0;

var uploadFiles = new Array();

function initFrontend(ws_url)
{
	wsURL = ws_url;

	$( document ).ready(function() {
		// Setup actions
		$('#connect_btn').click(connect);
		$('#compose_btn').click(function(evt){ $('#composer').modal('toggle'); });
		$('#pos_btn').click(postPosition);
		
		// Setup folder navigation
		$('#inbox_tab').click(function(evt){ displayFolder("in") });
		$('#outbox_tab').click(function(evt){ displayFolder("out") });
		$('#sent_tab').click(function(evt){ displayFolder("sent") });
		$('#archive_tab').click(function(evt){ displayFolder("archive") });

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

		initConsole();
		displayFolder("in");
	});

	updateStatusLoop();
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
	form.append('<input type="hidden" name="date" value="' + d + '" />');

	form.submit();

	$("#postiframe").load(function () {
		iframeContents = this.contentWindow.document.body.innerHTML;
		$('#composer').modal('hide');
		displayFolder(currentFolder);
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
    div.append('<p class="navbar-text">' + msg + '</p>');
    div.show();
	window.setTimeout(function() { div.fadeOut(500); }, 5000);
}

function updateStatusLoop()
{
	window.setTimeout(function() { updateStatusLoop(); }, 200);
	if(wsError) {
		$('#status_text').empty().append("<strong>Backend unavailable</strong>");
		return;
	}

	$.getJSON("/api/status", function(data){
		var st = $('#status_text');
		st.empty();

		if(data.connected){
			st.append("Connected " + data.remote_addr + "");
		} else if(data.active_listeners.length > 0){
			st.append("<i>Listening " + data.active_listeners + "</i>");
		}
	}).error(function(){
		st.append("<strong>Status error</strong>");
	})
}

function closeComposer(clear)
{
	if(clear){
		$('#msg_body').val('')
		$('#msg_subject').val('')
		$('#msg_to').val('')
	}
	$('#composer').modal('hide');
}

function connect(evt)
{
	connectStr = encodeURIComponent($('#connect_input').val());
	$('#connectModal').modal('hide');
	$.getJSON("/api/connect/" + connectStr, function(data){
		if( data.NumReceived > 0 ){
			displayFolder(currentFolder);
		} else {
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
		ws.onmessage = function(evt) { updateConsole(evt.data); };
		ws.onclose   = function(evt) {
			wsError = true;
			window.setTimeout(function() { initConsole(); }, 1000);
		};
	} else {
		// The browser doesn't support WebSocket
		wsError = true;
		alert("Websocket not supported by your browser. Console output will be unavailable.");
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
		+ "<th>Date</th><th>MID</th></tr></thead><tbody></tbody>");

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
				tbody.children().attr('class', 'active');
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

		view.find('#body').html('<p>' + data.Body + '</p>');

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
		view.show();
		$('#message_view').modal('show');
		if(!data.Read) {
			window.setTimeout(function() { setRead(currentFolder, data.MID); }, 2000);
		}
	});
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
