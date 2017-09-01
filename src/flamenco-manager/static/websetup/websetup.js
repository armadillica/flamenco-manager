function linkRequired() {
    var $result = $('#link-check-result');

    return $.get('/setup/api/link-required')
    .done(function(response) {
        console.log(response);
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
    .fail(function(err) {
        $result.text("Link check failed: " + err.responseText);
        showLinkButton();
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
        console.log(response);
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

function saveDataTables() {
    var variables = $('#variables-table').dataTable();
    var path_variables = $('#path-variables-table').dataTable();

    $('#variables-field').val(JSON.stringify(variables));
    $('#path-variables-field').val(JSON.stringify(path_variables));

    return true;
}


// Stuff to run on every "page ready" event.
$(document).ready(function() {
    linkRequired();

    // Source: https://codepen.io/ashblue/pen/mCtuA
    $('.table-add').click(function() {
        var $parent = $(this).closest('.table-editable');
        var $clone = $parent.find('tr.hide').clone(true).removeClass('hide table-line');
        $parent.find('table').append($clone);
    });

    $('.table-remove').click(function() {
        $(this).parents('tr').detach();
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
        // console.log(JSON.stringify(data));
    };
});
