var source = new EventSource('/imagewatch');

source.addEventListener('image', function (event) {
    console.log(event);
}, false);

/*
source.onopen = function () {
    console.log('Connection opened');
};

source.onerror = function () {
    console.log('Connection closed');
};

source.onmessage = function (event) {
    // a message without a type was fired
    console.log('Unknown message received', event)
};
*/
