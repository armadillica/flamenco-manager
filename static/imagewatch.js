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

function loadLatestImage(filename) {
    let url = '/' + filename + '?' + new Date().getTime();
    $('#last_rendered_image').attr('src', url);
    $('body.imageviewer').css('background-image', 'url(' + url + ')');
}

function reloadLatestImage() {
    let $img = $('#last_rendered_image');
    $img.attr('src', $img.attr('src'));

    let $imgviewer = $('body.imageviewer');
    $imgviewer.css('background-image', $imgviewer.css('background-image'));
}

// Global variable to ensure the EventSource isn't garbage collected.
var latestImageSource = null;

function createLatestImageListener() {
    if (latestImageSource != null) {
        latestImageSource.close();
    }

    let source = new EventSource('/imagewatch');

    source.addEventListener('image', function (event) {
        let filename = event.data;
        loadLatestImage(filename);
    }, false);

    source.onerror = obtainJWTToken;

    // Prevent errors when navigating away or reloading the page.
    window.addEventListener("beforeunload", function() {
        source.close();
    }, false);

    reloadLatestImage();

    latestImageSource = source;
    return source;
}

$(function () {
    createLatestImageListener();
    // Recreate the EventSource when there is a new JWT token.
    window.addEventListener("newJWTToken", createLatestImageListener);

    obtainJWTToken();
});
