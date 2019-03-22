/* Request a restart of Flamenco Manager.
 *
 * restartURL is the URL we should call to request the restart (which
 * is different for restarting to normal or setup mode).
 */
function restart(restartURL) {
    toastr.remove();
    toastr.info("Requesting restart");

    $.jwtAjax({
        method: 'POST',
        url: restartURL
    })
    .then(info => {
        toastr.options.progressBar = true;
        toastr.options.hideDuration = 200;
        toastr.options.onHidden = function() {
            window.location = "/setup";
        }
        toastr.remove();
        toastr.success("Flamenco Server is restarting", "Restarting", {timeOut: 2500});
    })
    .catch(error => {
        toastr.error(error.responseText, "Error " + error.status + " requesting a restart");
    })
    ;
}
