var source = new EventSource('/imagewatch');

source.addEventListener('image', function (event) {
    var filename = event.data;
    console.log(filename);

    var url = '/static/' + filename + '?' + new Date().getTime();
    $('#last_rendered_image').attr('src', url);
    $('body.imageviewer').css('background-image', 'url(' + url + ')');
}, false);
