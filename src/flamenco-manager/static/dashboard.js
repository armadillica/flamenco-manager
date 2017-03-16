
function load_workers() {
    var status_to_panel_class = {
        'awake': 'panel-success',
        'down': 'panel-default',
        'timeout': 'panel-danger',
    }

    $.get('/as-json')
    .done(function(info) {
        // Construct the overall status view.
        var $section = $('#status');
        $section.html('');

        var $dl = $('<dl>').addClass('dl-horizontal');
        $dl.append($('<dt>').text('Nr. of workers'));
        $dl.append($('<dd>').text(info.nr_of_workers));
        $dl.append($('<dt>')
            .text('Nr. of tasks')
            .attr('title', 'Number of tasks in database. Probably not all queued.'));
        $dl.append($('<dd>').text(info.nr_of_tasks));
        $dl.append($('<dt>').text('Server'));
        $dd = $('<dd>');
        $dd.append($('<a>').attr('href', info.server).text(info.server));
        $dl.append($dd);
        $section.append($dl);

        // Construct the worker list.
        var $section = $('#workers');
        $section.html('');

        var $row;
        var idx = 0;

        for (worker of info.workers) {
            // Start a new row.
            if (idx % 3 == 0) {
                var $row = $('<div>').addClass('row');
                $section.append($row);
            }
            idx++;

            var $div = $('<div>')
                .attr('id', worker._id)
                .addClass('col-md-4');
            $row.append($div);

            var $panel = $('<div>').addClass('panel');
            var panelclass = status_to_panel_class[worker.status];
            if (typeof panelclass == 'undefined') panelclass = 'panel-default';
            $panel.addClass(panelclass);
            $div.append($panel);

            var $header = $('<div>').addClass('panel-heading');
            $header.append($('<h3>')
                .text(worker.nickname)
                .addClass('panel-title')
            )
            $panel.append($header);

            var $dl = $('<dl>').addClass('dl-horizontal');
            // $dl.append($('<dt>').text('Nickname'));
            // $dl.append($('<dd>').text(worker.nickname));
            $dl.append($('<dt>').text('ID'));
            $dl.append($('<dd>').text(worker._id));
            $dl.append($('<dt>').text('Address'));
            $dl.append($('<dd>').text(worker.address));
            $dl.append($('<dt>').text('Status'));
            $dl.append($('<dd>').text(worker.status || '-none-').addClass('status-' + worker.status));

            var software = '-unknown-';
            if (worker.software) {
                /* 'Flamenco-Worker' is the default software, so don't mention that;
                 * do keep the version number, though. */
                software = worker.software.replace('Flamenco-Worker/', '');
            }
            $dl.append($('<dt>').text('Software'));
            $dl.append($('<dd>').text(software));
            // $dl.append($('<dt>').text('Platform'));
            // $dl.append($('<dd>').text(worker.platform));

            $dl.append($('<dt>').text('Cur/last Task'));
            $task_dd = $('<dd>');
            if (typeof worker.current_task != 'undefined') {
                $tasklink = $('<a>')
                    .attr('href', info.server + 'flamenco/tasks/' + worker.current_task)
                    .text(worker.current_task);
                $task_dd.append($tasklink);
            } else {
                $task_dd.text('-none-');
            }
            $dl.append($task_dd);

            var strdiff;
            var last_act = new Date(worker.last_activity);
            if (typeof worker.last_activity == 'undefined') {
                strdiff = '-nevah-';
            } else {
                var timediff = Date.now() - last_act;  // in milliseconds
                if (timediff < 1000) {
                    strdiff = 'just now';
                } else if (timediff < 60000) {  // less than a minute
                    strdiff = Math.round(timediff / 1000) + ' seconds ago';
                } else if (timediff < 3600000) { // less than an hour
                    strdiff = Math.round(timediff / 60000) + ' minutes ago';
                } else if (timediff < 24 * 3600000) { // less than a day hour
                    strdiff = Math.round(timediff / 3600000) + ' hours ago';
                } else {
                    strdiff = last_act;
                }
            }
            $dl.append($('<dt>').text('Last Seen'));
            $dl.append($('<dd>')
                .text(strdiff)
                .attr('title', last_act));
            // $dl.append($('<dt>').text('Supported Job Types'));
            // $dl.append($('<dd>').text(worker.supported_job_types.join(', ')));

            var $panel_body = $('<div>').addClass('panel-body');
            $panel_body.append($dl);
            $panel.append($panel_body);
        }

        // Everything went fine, let's try it again soon.
        setTimeout(load_workers, 2000);
    })
    .fail(function(error) {
        var $section = $('#status');
        var $p = $('<p>').addClass('error');

        if (error.status) {
            $p.text('Error ' + error.status + ': ' + error.responseText);
        } else {
            $p.text('Unable to get the status report. Is the Manager still running & reachable?');
        }
        $section.html($p);

        // Everything is bugging out, let's try it again soon-ish.
        setTimeout(load_workers, 10000);
    });
}

$(load_workers);
