var source = new EventSource('/imagewatch');

source.addEventListener('image', function (event) {
    var filename = event.data;
    if (console) console.log(filename);

    var url = '/static/' + filename + '?' + new Date().getTime();
    $('#last_rendered_image').attr('src', url);
    $('body.imageviewer').css('background-image', 'url(' + url + ')');
}, false);

// For debugging purposes this can be handy:
// source.addEventListener('notification', function (event) {
//     console.log("Received notification from image watcher:", event.data);
// }, false);
