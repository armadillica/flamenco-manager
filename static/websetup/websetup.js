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

let EMPTY_VARIABLE = Object.freeze({
    value: '',
    audience: '',
    platform: '',
});
let SUPPORTED_PLATFORMS = ['linux', 'darwin', 'windows'];


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

let eventBus = new Vue();

Vue.component('setup-form', {
    props: {
        own_urls: Array,
        config: Object,
        original_config: Object,
        mongo_choice: String,
        hide_infra_settings: Boolean,
    },
    template: "#template_setup_form",
    computed: {
        saveButtonDisabled() {
            // Unfortunately JavaScript has no proper object comparison operator...
            return JSON.stringify(this.config) == JSON.stringify(this.original_config);
        },
    },
    methods: {
        saveContent(restartAfterSaving) {
            this.$emit('configupdated', {
                config: this.config,
                restartAfterSaving: !!restartAfterSaving,
                restartTo: 'setup',
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
                restartTo: 'normal',
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

// Editor for multiple variables.
Vue.component('variables-editor', {
    props: {
        variables: Object,
    },
    template: '#template_variables_editor',
    methods: {
        removeVariable(variableName) {
            console.log("requested removal of variable", variableName);
            Vue.delete(this.variables, variableName);
        },
    },
});

// Editor for a single variable.
Vue.component('variable-editor', {
    props: {
        varName: String,
        varDef: Object, // variable definition
    },
    data() { return {
        allAudience: 'all',
    }},
    template: '#template_variable_editor',
    created() {
        let reducer = (acc, varValue) => { return acc && varValue.audience == 'all' };
        let isAllAudience = this.varDef.values.reduce(reducer, true);
        this.allAudience = isAllAudience ? 'all' : 'separate';

        // Before the configuration is saved, we need to clean up some stuff.
        eventBus.$on('pre-save-config', this.cleanupAudiences);
    },
    computed: {
        showAudienceRow() {
            return this.allAudience == 'separate';
        },
        disableAddValueButton() {
            return this.propsForNewValue == null;
        },
        propsForNewValue() {
            // Returns some sensible properties for a new variable value, or null if there is none.
            let isAllAudience = this.allAudience == 'all';
            let platformsLeft = {
                users: new Set(SUPPORTED_PLATFORMS),
                workers: new Set(SUPPORTED_PLATFORMS),
            };

            // See which platforms and audiences already have values.
            for (var value of this.varDef.values) {
                if (isAllAudience || value.audience == "all") {
                    platformsLeft.workers.delete(value.platform);
                    platformsLeft.users.delete(value.platform);
                } else {
                    platformsLeft[value.audience].delete(value.platform);
                }
            }

            // Return the first audience+platform combo that's left.
            for (audience in platformsLeft) {
                // There is no Set.prototype.pop() to get any item from the set; the iterator is the only way to access values.
                for (var platform of platformsLeft[audience].values()) {
                    return {
                        audience: isAllAudience ? 'all' : audience,
                        platform: platform,
                    }
                }
            }

            return null;
        },
    },
    methods: {
        remove() {
            this.$emit("removeVariable", this.varName);
        },
        removeValue(valueIndex) {
            // splice() returns the removed item as a one-element array.
            // We could use this to make a little undo system.
            this.varDef.values.splice(valueIndex, 1);
        },
        addValue() {
            let newPartialValue = this.propsForNewValue;
            let newVariableValue = JSON.parse(JSON.stringify(EMPTY_VARIABLE));
            if (newPartialValue != null) Object.assign(newVariableValue, newPartialValue);
            this.varDef.values.push(newVariableValue);
        },
        cleanupAudiences() {
            // To allow users to toggle losslessly between "separate" and "all" audiences,
            // we don't set the variable values' audience immediately. This is done in this
            // function just before saving.
            if (this.allAudience == 'separate') return;

            for (var varValue of this.varDef.values) {
                varValue.audience = 'all';
            }
        },
    },
});

// Toggle slider with a left-side and right-side label.
Vue.component('two-way-toggle', {
    props: {
        name: String,
        labelLeft: String,
        labelRight: String,
        valueLeft: String,
        valueRight: String,
        value: String,
    },
    template: '#template_two_way_toggle',
    computed: {
        toggleID() {
            return 'two_way_toggle_' + this.name;
        },
        isChecked() {
            return this.value == this.valueRight;
        },
    },
    methods: {
        onInput(event) {
            /* This creates an 'input' event that's caught by the `v-model` attribute
             * of our parent component. It's that component that changes our `value`
             * property; we don't have to do that ourselves. */
            let value = event.target.checked ? this.valueRight : this.valueLeft;
            this.$emit('input', value);
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
                this.markConfigAsOriginal();
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
        },
        markConfigAsOriginal() {
            // Keep a deep copy around so that we can go back to unchanged values.
            this.original_config = JSON.parse(JSON.stringify(this.config));
        },

        saveConfig(options) {
            this.setConfig(options.config);

            // Allow components to do pre-save cleanup.
            eventBus.$emit('pre-save-config');

            // Get rid of Vue.js specific getters/setters and __ob__ properties.
            // The JSON dumper is less sensitive to this than the YAML dumper.
            let configToSave = JSON.parse(JSON.stringify(options.config));

            $.jwtAjax({
                method: 'POST',
                url: '/setup/data',
                data: jsyaml.safeDump(configToSave),
                headers: {'Content-Type': 'application/x-yaml'},
            })
            .then(() => {
                this.markConfigAsOriginal();
                if (!options.restartAfterSaving) {
                    toastr.success("Restart Flamenco Manager to apply the new settings.", "Configuration saved");
                    return;
                }
                if (typeof options.restartTo == 'undefined' || options.restartTo == 'setup') {
                    restart("/setup/restart-to-setup");
                } else if (options.restartTo == 'normal') {
                    restart("/setup/restart");
                } else {
                    console.log("Unknown 'restartTo' option: ", options.restartTo);
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
