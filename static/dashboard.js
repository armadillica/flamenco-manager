var clipboard;

// So we can call window.clearTimeout(load_workers_timeout_handle)
// from the browser's JS console.
var load_workers_timeout_handle;

Vue.component('status', {
    props: ['serverinfo', 'errormsg', 'idle_workers'],
    template: '#template_status',
    methods: {
        forgetWorker(worker) {
            workerAction(worker._id, { action: 'forget-worker' },
                "Are you sure you want to erase " + worker.nickname + "?\n\n" +
                "This will cause authentication errors on the Worker if you " +
                "ever restart it; use --reregister on the worker to solve that.")
        },
    },
})

Vue.component('idle-worker', {
    props: {
        worker: Object,
    },
    template: '#template_idle_worker',
})

Vue.component('action-bar', {
    props: {
        workers: Array,
        selected_worker_ids: Array,
    },
    template: '#template_action_bar',
    data: function() {
        return {
            selected_action: '',

            // Freeze to prevent Vue.js from creating getters & setters all over this object.
            // We don't need it to be tracked, as it won't be changed anyway.
            actions: Object.freeze({
                offline: {
                    label: '✝ Shut Down',
                    title: 'The worker may automatically restart.',
                    payload: { action: 'shutdown' },
                },
                asleep: {
                    label: '😴 Send to Sleep',
                    title: 'Let the worker sleep',
                    payload: { action: 'set-status', status: 'asleep' },
                },
                wakeup: {
                    label: '😃 Wake Up',
                    title: 'Wake the worker up. A sleeping worker can take a minute to respond.',
                    payload: { action: 'set-status', status: 'awake' },
                },
                ack_timeout: {
                    label: '✓ Acknowledge Timeout',
                    payload: { action: 'ack-timeout' },
                },
                testjob: {
                    label: 'Send a Test Job',
                    title: 'Requires the worker to be in test mode.',
                    payload: { action: 'send-test-job' },
                },
            }),
        };
    },
    methods: {
        performWorkerAction: function() {
            let action = this.actions[this.selected_action];
            let payload = action.payload;

            for(worker_id of this.selected_worker_ids) {
                workerAction(worker_id, payload);
            }
        }
    }
})

Vue.component('worker-row', {
    props: {
        worker: Object,
        selected_worker_ids: Array,
    },
    template: '#template_worker_row',
    computed: {
        checkbox_id: function () {
            return 'select_' + this.worker._id;
        },
        task_id_text: function () {
            return this.worker.current_task.substr(-8);
        },
        task_log_url: function () {
            return '/logfile/' + this.worker.current_job + '/' + this.worker.current_task;
        },
        last_activity_abs: function () {
            return new Date(this.worker.last_activity);
        },
        worker_software: function () {
            if (this.worker.software) {
                /* 'Flamenco-Worker' is the default software, so don't mention that;
                 * do keep the version number, though. */
                return this.worker.software.replace('Flamenco-Worker/', '');
            }
            return '-unknown-';
        },
        is_checked: function() {
            return this.selected_worker_ids.indexOf(this.worker._id) >= 0;
        },
    },
    methods: {
        current_task_updated: function () {
            return time_diff(this.worker.current_task_updated);
        },
        last_activity_rel: function () {
            return time_diff(this.worker.last_activity);
        },
    }
})

// Load the selection from local storage.
// This data is stored by the vueApp selected_worker_ids watch function.
function loadSelectedWorkers() {
    let workersAsJSON = localStorage.getItem('selected_worker_ids');
    if (!workersAsJSON) return [];

    try {
        return JSON.parse(workersAsJSON) || [];
    } catch(ex) {
        localStorage.removeItem('selected_worker_ids');
        return [];
    }
}


var vueApp = new Vue({
    el: '#vue_app',
    created() {
        this.loadWorkers();
    },
    data: {
        errormsg: '',
        serverinfo: {
            nr_of_workers: 0,
            nr_of_tasks: 0,
            upstream_queue_size: 0,
            version: "unknown",
            server: {
                name: "unknown",
                url: "#",
            },
        },
        idle_workers: [],
        current_workers: [],
        selected_worker_ids: loadSelectedWorkers(),
    },
    methods: {
        loadWorkers() {
            window.clearTimeout(load_workers_timeout_handle);

            $.get('/as-json')
                .done(info => {
                    this.errormsg = '';

                    // Only copy those keys we've declared in the serverinfo object.
                    for (key in info) {
                        if (!info.hasOwnProperty(key)) continue;
                        if (!this.serverinfo.hasOwnProperty(key)) continue;
                        this.serverinfo[key] = info[key];
                    }

                    // Split workers into "current" and "idle for too long"
                    let too_long = 14 * 24 * 3600000; // in milliseconds
                    let idle_workers = [];
                    let current_workers = [];
                    let selectable_worker_ids = new Set();
                    if (typeof info.workers != 'undefined' && info.workers != null) {
                        for (worker of info.workers) {
                            if (typeof worker.last_activity == 'undefined') {
                                idle_workers.push(worker);
                                continue;
                            }

                            let as_date = new Date(worker.last_activity);
                            let timediff = Date.now() - as_date;  // in milliseconds
                            if (timediff > too_long) {
                                idle_workers.push(worker);
                            } else {
                                current_workers.push(worker);
                                // Only current workers get a select box; a worker moving to 'idle' status
                                // shouldn't keep it selected (because we can't even unselect it).
                                selectable_worker_ids.add(worker._id);
                            }
                        }
                    }
                    this.idle_workers = idle_workers;
                    this.current_workers = current_workers;

                    // Deselect all non-selectable workers. We should be able to iterate over all selected
                    // workers later, and not worry about them not existing any more.
                    this.selected_worker_ids = this.selected_worker_ids.filter(id => selectable_worker_ids.has(id));

                    // Everything went fine, let's try it again soon.
                    load_workers_timeout_handle = setTimeout(this.loadWorkers, 2000);
                })
                .fail(error => {
                    if (error.status) {
                        this.errormsg = 'Error ' + error.status + ': ' + error.responseText;
                    } else {
                        this.errormsg = 'Unable to get the status report. Is the Manager still running & reachable?';
                    }

                    // Everything is bugging out, let's try it again soon-ish.
                    load_workers_timeout_handle = setTimeout(this.loadWorkers, 10000);
                })
                .always(function () {
                    if (typeof clipboard != 'undefined') {
                        clipboard.destroy();
                    }
                    clipboard = new Clipboard('.click-to-copy');
                    clipboard.on('success', function (e) {
                        $(e.trigger).flashOnce();
                    });

                    $('.click-to-copy').attr('title', 'Click to copy');
                })
                ;
        },

        onWorkerSelected(is_selected, worker_id) {
            let selected_ids = new Set(this.selected_worker_ids);

            if (is_selected) selected_ids.add(worker_id);
            else selected_ids.delete(worker_id);

            this.selected_worker_ids = Array.from(selected_ids);
        },
    },
    watch: {
        selected_worker_ids(worker_ids) {
            if (worker_ids.length > 0) {
                localStorage.setItem('selected_worker_ids', JSON.stringify(worker_ids));
            } else {
                localStorage.removeItem('selected_worker_ids');
            }
        },
    },
});


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
}


/* Perform a worker action like 'forget-worker', 'shutdown', etc.
 * See dashboard.go, function workerAction() */
function workerAction(workerID, payload, confirmation) {
    if (typeof confirmation !== 'undefined' && !confirm(confirmation)) return;

    $.post('/worker-action/' + workerID, payload)
        .done(function (resp) {
            if (!resp) resp = "Request confirmed"
            toastr.success(resp);
            vueApp.loadWorkers();
        })
        .fail(function (resp) {
            console.log(resp);
            toastr.error(resp.responseText);
        })
        ;
}

function downloadkick() {
    var button = $(this);
    button.fadeOut();
    $.get('/kick')
        .done(function () {
            button.fadeIn();
        })
        .fail(function (err) {
            button.text('Error, see console.');
            button.fadeIn();
            console.log(err);
        });
}


$.fn.flashOnce = function () {
    var target = this;
    this
        .addClass('flash-on')
        .delay(500) // this delay is linked to the transition in the flash-on CSS class.
        .queue(function () {
            target
                .removeClass('flash-on')
                .addClass('flash-off')
                .dequeue()
                ;
        })
        .delay(1000)  // this delay is just to clean up the flash-X classes.
        .queue(function () {
            target
                .removeClass('flash-on flash-off')
                .dequeue()
                ;
        })
        ;
    return this;
};

$(function () {
    toastr.options.closeButton = true;
    toastr.options.progressBar = true;
    toastr.options.positionClass = 'toast-bottom-left';
    toastr.options.hideMethod = 'slideUp';

    $('#downloadkick').on('click', downloadkick);
})
