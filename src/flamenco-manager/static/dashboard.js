
function load_workers() {
    $.get('/as-json')
    .done(function(info) {
        // Construct the overall status view.
        var $section = $('#status');
        $section.html('');

        var $dl = $('<dl>');
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
        for (worker of info.workers) {
            var $div = $('<div>')
                .attr('id', worker._id)
                .addClass('worker-info worker-status-' + worker.status);
            $div.append($('<h3>').text(worker.nickname))

            var $dl = $('<dl>');
            // $dl.append($('<dt>').text('Nickname'));
            // $dl.append($('<dd>').text(worker.nickname));
            // $dl.append($('<dt>').text('ID'));
            // $dl.append($('<dd>').text(worker._id));
            // $dl.append($('<dt>').text('Address'));
            // $dl.append($('<dd>').text(worker.address));
            $dl.append($('<dt>').text('Status'));
            $dl.append($('<dd>').text(worker.status).addClass('status-' + worker.status));
            // $dl.append($('<dt>').text('Platform'));
            // $dl.append($('<dd>').text(worker.platform));

            $dl.append($('<dt>').text('Current Task'));
            $task_dd = $('<dd>');
            if (typeof worker.current_task != 'undefined') {
                $tasklink = $('<a>').attr('href', info.server + '/flamenco/tasks/' + worker.current_task);
                $task_dd.append(tasklink);
            } else {
                $task_dd.text('-none-');
            }
            $dl.append($task_dd);

            var last_act = new Date(worker.last_activity);
            var timediff = Date.now() - last_act;  // in milliseconds
            var strdiff;
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
            $dl.append($('<dt>').text('Last Seen'));
            $dl.append($('<dd>')
                .text(strdiff)
                .attr('title', last_act));
            // $dl.append($('<dt>').text('Supported Job Types'));
            // $dl.append($('<dd>').text(worker.supported_job_types.join(', ')));
            $div.append($dl);

            $section.append($div);
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
