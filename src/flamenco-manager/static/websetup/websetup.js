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
            $('#relink-button').show();
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
    $('#relink-button').hide();
}

// Stuff to run on every "page ready" event.
$(document).ready(function() {
    linkRequired();
    $('#relink-button').click(showLinkButton);
    // $('#link-button').click(linkButtonClicked);
});
