function showLinkButton() {
    $('#link-button').fadeIn();
}

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
        }
    })
    .fail(function(err) {
        $result.text("Link check failed: " + err.responseText);
        showLinkButton();
    })
    .always(function() {
        $('#link-check-in-progress').hide();
    })

    ;
}

// Stuff to run on every "page ready" event.
$(document).ready(function() {
    linkRequired();
});
