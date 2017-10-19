var clipboard;
var load_workers_timeout_handle;

function show_action(action_status, worker) {
    if (worker.status == action_status && worker.status_requested == '')
        return false;
    if (worker.status_requested == action_status)
        return false;
    return true;
}

function load_workers() {
    window.clearTimeout(load_workers_timeout_handle);

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
        $dd.append($('<a>').attr('href', info.server + 'flamenco/').text(info.server));
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
        var $tbody = $('<tbody id="workers">');
        for (worker of current_workers) {
            var $row = $('<tr>')
                .attr('id', worker._id)
                .addClass("status-" + worker.status);

            var actionrow = $('<td>');
            if (show_action('asleep', worker)) {
                actionrow.append($('<a>').workerAction(worker._id, {
                        action: 'set-status',
                        status: 'asleep',
                    })
                    .text('😴')
                    .attr('title', 'Let the worker sleep')
                );
            }
            if (show_action('awake', worker)) {
                actionrow.append($('<a>').workerAction(worker._id, {
                        action: 'set-status',
                        status: 'awake',
                    })
                    .text('😃')
                    .attr('title', 'Wake the worker up')
                );
            }
            $row.append(actionrow);

            $row.append($('<td>').text(worker.nickname));

            var status_text = worker.status || '-none-';
            if (worker.status_requested) {
                status_text += ' → ' + worker.status_requested;
            }
            $row.append($('<td>').text(status_text).addClass('status-' + worker.status));

            $task_td = $('<td>');
            if (typeof worker.current_task != 'undefined') {
                var task_id_text = '…' + worker.current_task.substr(-8);
                var task_text;
                if (typeof worker.current_task_status != 'undefined') {
                    if (typeof worker.current_task_updated != 'undefined') {
                        task_text = task_id_text + " (" + worker.current_task_status + " " + time_diff(worker.current_task_updated) + ")";
                    } else {
                        task_text = task_id_text + " (" + worker.current_task_status + ")";
                    }
                } else {
                    task_text = task_id_text;
                }
                $tasklink = $('<a>')
                    .attr('href', info.server + 'flamenco/tasks/' + worker.current_task)
                    .text(task_text);
                $task_td.append($tasklink);
            } else {
                $task_td.text('-none-');
            }
            $row.append($task_td);

            $row.append($('<td>')
                .text(time_diff(worker.last_activity))
                .attr('title', new Date(worker.last_activity)));
            // $dl.append($('<dd>').text(worker.supported_job_types.join(', ')));
            $row.append($('<td>')
                .addClass('click-to-copy worker-id')
                .attr('data-clipboard-text', worker._id)
                .text('…' + worker._id.substr(-6))
            );
            $row.append($('<td>')
                .addClass('click-to-copy worker-address')
                .attr('data-clipboard-text', worker.address)
                .text(worker.address));

            var software = '-unknown-';
            if (worker.software) {
                /* 'Flamenco-Worker' is the default software, so don't mention that;
                 * do keep the version number, though. */
                software = worker.software.replace('Flamenco-Worker/', '');
            }
            $row.append($('<td>').text(software));
            // $row.append($('<td>').text(worker.platform));

            $tbody.append($row);
        }
        $('#workers').replaceWith($tbody);

        // Everything went fine, let's try it again soon.
        load_workers_timeout_handle = setTimeout(load_workers, 2000);
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
        load_workers_timeout_handle = setTimeout(load_workers, 10000);
    })
    .always(function() {
        if (typeof clipboard != 'undefined') {
            clipboard.destroy();
        }
        clipboard = new Clipboard('.click-to-copy');

        $('.click-to-copy').attr('title', 'Click to copy');
    })
    ;
}

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
    if (timediff < 2 * 24 * 3600000) { // less than two days
        return Math.round(timediff / 3600000) + ' hours ago';
    }

    return as_date.toLocaleString('en-GB', {
        hour12: false,
        year: 'numeric',
        month: 'short',
        day: 'numeric',
    });
    return (1900 + as_date.getYear()) + '-' + as_date.getMonth() + '-' + as_date.getDay();
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
    toastr.options.closeButton = true;
    toastr.options.progressBar = true;
    toastr.options.positionClass = 'toast-bottom-left';
    toastr.options.hideMethod = 'slideUp';

    $.fn.workerAction = function(workerID, payload) {
        this.click(function() {
            $.post('/worker-action/' + workerID, payload)
            .done(function(resp) {
                if (!resp) resp = "Request confirmed"
                toastr.success(resp);
                load_workers();
            })
            .fail(function(resp) {
                console.log(resp);
                toastr.error(resp.responseText);
            })
            ;
        })
        this.addClass('worker-action');
        return this;
    }

    load_workers();
    $('#downloadkick').on('click', downloadkick);
})
