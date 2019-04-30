/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 *
 * This file is part of Flamenco Manager.
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 * ***** END MIT LICENCE BLOCK *****
 */

function linkRequired() {
    var $result = $('#link-check-result');

    $.get('/setup/api/link-required')
    .fail(error => {
        if (error.status == 401 || error.status == 498) {
            window.addEventListener("newJWTToken", linkRequired);
            obtainJWTToken()
            .then(linkRequired)
            .catch(function(err) {
                showLinkButton();

                if (err.getResponseHeader("Content-Type").startsWith("text/html") && err.responseText) {
                    $result.html("Link check failed auth check: " + err.responseText);
                } else {
                    var errorMsg = err.responseText || "unable to connect to Flamenco Server";
                    $result.text("Link check failed auth check: " + errorMsg);
                }
            });
            return;
        }
        showLinkButton();
        $result.text("Link check failed: " + error.responseText);
    })
    .done(function(response) {
        window.removeEventListener("newJWTToken", linkRequired);

        if (response.link_required) {
            $result.text("You need to link this Flamenco Manager to a Flamenco Server.");
            showLinkButton();
        } else {
            var $link = $("<a>")
                .attr("href", response.server_url)
                .text(response.server_url)
            $result
                .text("This Flamenco Manager is linked to a Flamenco Server at ")
                .append($link);
            $('#relink-action').show();
        }
    })
    .always(function() {
        $('#link-check-in-progress').remove();
    })
    ;
}

// Starts the linking process when someone clicks on the link button.
function linkButtonClicked() {
    var $result = $('#link-start-result');
    $result.html('');

    $.get(
        "/setup/api/link-start",
        {
            server: $('#link-server-url').val()
        }
    )
    .done(function(response) {
        // We received an URL to direct the user to.
        var $link = $('<a>')
            .attr('href', response.location)
            .text('your Flamenco Server');
        $result
            .text("Redirecting to ")
            .append($link)
            .show();
        window.location = response.location;
    })
    .fail(function(err) {
        $result
            .text("Linking could not start: " + err.responseText)
            .show();
    })
    ;
}

function showLinkButton() {
    $('#link-button').show();
    $('#link-form').show();
    $('#relink-action').hide();
}

function hideLinkButton() {
    $('#link-button').hide();
    $('#link-form').hide();
    $('#relink-action').show();
}

function saveDataTables() {
    var variables = $('#variables-table').dataTable();
    var path_variables = $('#path-variables-table').dataTable();

    // When adding a new variable its name is set to "variable-name".
    // If this is still there, the user probably clicked the '+' by accident
    // and didn't remove the example variable.
    let removeDefault = function(someVarInfo) {
        return someVarInfo.name != "variable-name";
    }
    variables = variables.filter(removeDefault);
    path_variables = path_variables.filter(removeDefault);

    $('#variables-field').val(JSON.stringify(variables));
    $('#path-variables-field').val(JSON.stringify(path_variables));

    return true;
}

Vue.component('setup-form', {
    props: {
        own_urls: Array,
        config: Object,
        original_config: Object,
        mongo_choice: String,
    },
    template: "#template_setup_form",
    methods: {
        saveContent(restartAfterSaving) {
            this.$emit('configupdated', {
                config: this.config,
                restartAfterSaving: !!restartAfterSaving,
            });
        },
    },
});

Vue.component('yaml-editor', {
    props: {
        config: Object,
        original_config: Object,
    },
    data() { return {
        editor: null,
    }},
    template: "#template_yaml_editor",
    created() {
        this.$nextTick(this.createEditor);
    },
    destroyed() {
        this.editor.destroy();
        this.editor.container.remove();
    },
    watch: {
        config(newConfig, oldConfig) {
            let curpos = this.editor.selection.getCursor();
            this.editor.setValue(this.yaml);
            this.editor.gotoLine(curpos.row+1);
        },
    },
    computed: {
        yaml() {
            return jsyaml.safeDump(this.config);
        },
    },
    methods: {
        createEditor() {
            this.editor = ace.edit("ace_yaml_editor");
            this.editor.setTheme("ace/theme/github");
            this.editor.session.setMode("ace/mode/yaml");
            this.editor.setOption("fontSize", "4mm");
        },
        saveContent(restartAfterSaving) {
            var config;
            try {
                config = jsyaml.safeLoad(this.editor.getValue());
            } catch (ex) {
                if (ex.name != 'YAMLException') throw ex;

                toastr.error(ex.reason, "Error parsing YAML:");
                this.editor.gotoLine(ex.mark.line);
                return;
            }
            this.$emit('configupdated', {
                config: config,
                restartAfterSaving: !!restartAfterSaving,
            });
        },
        restart(restartURL) {
            restart(restartURL);
        },
    },
});


Vue.component('input-field', {
    props: {
        id_input: String,
        placeholder: String,
        label: String,
        help: String,
        value: String,
        original_value: String,
    },
    template: '#template_input_field',
    computed: {
        restore_title: function () {
            return 'Restore Saved Value: ' + this.original_value;
        },
    },
});

Vue.component('checkbox-field', {
    props: {
        id_input: String,
        label: String,
        label2: String,
        help: String,
        value: Boolean,
        original_value: Boolean,
    },
    template: '#template_checkbox_field',
    computed: {
        restore_title: function () {
            return 'Restore Saved Value: ' + this.original_value;
        },
    },
});


var vueApp = new Vue({
    el: '#vueApp',
    data: {
        own_urls: [],
        config: {},
        original_config: {},
        mongo_choice: "builtin",
        vueAppMode: "form",   // or "yaml", see switchAppMode().
    },
    created() {
        this.loadSetupData();
    },
    computed: {
        appModeButtonText() {
            return {
                form: "Advanced",
                yaml: "Simple",
            }[this.vueAppMode];
        },
    },
    methods: {
        switchAppMode() {
            this.vueAppMode = {
                form: "yaml",
                yaml: "form",
            }[this.vueAppMode];
        },

        loadSetupData() {
            $.jwtAjax({
                method: 'GET',
                url: '/setup/data',
            })
            .then(response => {
                window.removeEventListener("newJWTToken", this.loadSetupData);
                let settings = jsyaml.safeLoad(response);

                this.own_urls = settings.own_urls;
                this.setConfig(settings.config);
            })
            .catch(error => {
                var title;
                let message = error.responseText || "Unable to connect to server.";
                if (error.requestStage == "JWT") {
                    title = "Unable to obtain authorization token";
                } else {
                    title = "Unable to load settings";
                }
                toastr.error(message, title);
                showLinkButton();
            })
            ;
        },

        setConfig(config) {
            this.config = config;
            this.mongo_choice = config.database_url == '' ? 'bundled' : 'external';

            // Keep a deep copy around so that we can go back to unchanged values.
            this.original_config = JSON.parse(JSON.stringify(config));
        },

        saveConfig(options) {
            this.setConfig(options.config);

            $.jwtAjax({
                method: 'POST',
                url: '/setup/data',
                data: jsyaml.safeDump(options.config),
                headers: {'Content-Type': 'application/x-yaml'},
            })
            .then(() => {
                if (options.restartAfterSaving) {
                    restart("/setup/restart-to-setup");
                } else {
                    toastr.success("Restart Flamenco Manager to apply the new settings.", "Configuration saved");
                }
            })
            .catch(error => {
                var title;
                let message = error.responseText || "Unable to connect to server.";
                if (error.requestStage == "JWT") {
                    title = "Unable to obtain authorization token";
                } else {
                    title = "Unable to save settings";
                }
                toastr.error(message, title);
            })
            ;
        },
    },
});

// Stuff to run on every "page ready" event.
$(document).ready(function() {
    linkRequired();

    // Source: https://codepen.io/ashblue/pen/mCtuA
    $('.table-add').click(function() {
        var $parent = $(this).closest('.table-editable');
        var $clone = $parent.find('tr.d-none').clone(true).removeClass('d-none table-line');
        $parent.find('table').append($clone);
    });

    $('.table-remove').click(function() {
        $(this).parents('tr').detach();
    });

    $(document).ajaxStart(function() {
        $('#loading-settings').show();
    });
    $(document).ajaxStop(function() {
        $('#loading-settings').hide();
    });

    // A few jQuery helpers for exporting only
    jQuery.fn.pop = [].pop;
    jQuery.fn.shift = [].shift;

    jQuery.fn.dataTable = function() {
        var $rows = this.find('tr:not(:hidden)');
        var headers = [];
        var data = [];

        // Get the headers
        $($rows.shift()).find('th:not(:empty)').each(function() {
            headers.push($(this).data('key'));
        });

        // Turn all existing rows into a loopable array
        $rows.each(function() {
            var $td = $(this).find('td');
            var h = {};

            // Use the headers from earlier to name our hash keys
            headers.forEach(function(header, i) {
                h[header] = $td.eq(i).text();
            });

            data.push(h);
        });

        // Output the result
        return data;
    };
});
