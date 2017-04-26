function load_workers() {
    var status_to_panel_class = {
        'awake': 'panel-success',
        'down': 'panel-default',
        'timeout': 'panel-danger',
    }

    $.get('/as-json')
    .done(function(info) {
        // Split workers into "current" and "idle for too long"
        var too_long = 14 * 24 * 3600000; // in milliseconds
        var current_workers = [];
        var idle_workers = [];

        $('#managerversion').text(info.version);

        // Make sure we can iterate over the list.
        if (typeof info.workers == 'undefined' || info.workers == null) {
            info.workers = [];
        }

        for (worker of info.workers) {
            var as_date = new Date(worker.last_activity);
            var timediff = Date.now() - as_date;  // in milliseconds
            if (typeof worker.last_activity == 'undefined' || timediff > too_long) {
                idle_workers.push(worker);
            } else {
                current_workers.push(worker);
            }
        }

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
        if (idle_workers.length > 0) {
            $dl.append($('<dt>')
                .text('Old workers')
                .attr('title', 'Workers not seen in over a month.'));
            $dd = $('<dd>');
            for (worker of idle_workers) {
                $dd.append($('<span>')
                    .addClass('idle-worker-name')
                    .text(worker.nickname)
                    .attr('title', worker._id)
                );
            }
            $dl.append($dd);
        }
        $section.append($dl);

        // Construct the worker list.
        var $section = $('#workers');
        $section.html('');

        var $row;
        var idx = 0;

        for (worker of current_workers) {
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
                var task_text;
                if (typeof worker.current_task_status != 'undefined') {
                    if (typeof worker.current_task_updated != 'undefined') {
                        task_text = worker.current_task + " (" + worker.current_task_status + " " + time_diff(worker.current_task_updated) + ")";
                    } else {
                        task_text = worker.current_task + " (" + worker.current_task_status + ")";
                    }
                } else {
                    task_text = worker.current_task;
                }
                $tasklink = $('<a>')
                    .attr('href', info.server + 'flamenco/tasks/' + worker.current_task)
                    .text(task_text);
                $task_dd.append($tasklink);
            } else {
                $task_dd.text('-none-');
            }
            $dl.append($task_dd);

            $dl.append($('<dt>').text('Last Seen'));
            $dl.append($('<dd>')
                .text(time_diff(worker.last_activity))
                .attr('title', new Date(worker.last_activity)));
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


function time_diff(timestamp) {
    if (typeof timestamp == 'undefined') {
        return '-nevah-';
    }

    var as_date = new Date(timestamp);
    var timediff = Date.now() - as_date;  // in milliseconds

    if (timediff < 1000) {
        return 'just now';
    }
    if (timediff < 60000) {  // less than a minute
        return Math.round(timediff / 1000) + ' seconds ago';
    }
    if (timediff < 3600000) { // less than an hour
        return Math.round(timediff / 60000) + ' minutes ago';
    }
    if (timediff < 24 * 3600000) { // less than a day hour
        return Math.round(timediff / 3600000) + ' hours ago';
    }

    return as_date;
}

function downloadkick() {
    var button = $(this);
    button.fadeOut();
    $.get('/kick')
    .done(function() {
        button.fadeIn();
    })
    .fail(function(err) {
        button.text('Error, see console.');
        button.fadeIn();
        console.log(err);
    });
}

$(function() {
    $('#downloadkick').on('click', downloadkick);
})
