/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
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

const jwtTokenCookieName = 'jwtToken';
const jwtTokenCookieOptions = {
    expires: 1,  // the token will be way shorter lived than 1 day.
    path: '/',
};
var _jwtToken = null;  // cache so that we don't have to round-trip to cookies all the time.

/* Return the JWT token, if we have gotten any. */
function jwtToken() {
    if (_jwtToken == null) {
        _jwtToken = Cookies.get(jwtTokenCookieName)
    }
    return _jwtToken;
}

/* Set a new JWT token. */
function setJWTToken(newToken) {
    _jwtToken = newToken;
    Cookies.set(jwtTokenCookieName, newToken, jwtTokenCookieOptions);

    // Let the world know there is a new token.
    if (newToken) {
        let event = new Event("newJWTToken");
        window.dispatchEvent(event);
    }
}

/* Forget the JWT token; only used for debugging purposes. */
function removeJWTToken() {
    Cookies.remove(jwtTokenCookieName, jwtTokenCookieOptions);
    _jwtToken = null;
    console.log('JWT token forgotten');
}

var jwtTokenObtainingPromise = null;

/* Fetch a new JWT token from Flamenco Server. */
function obtainJWTToken() {
    // If another GET to fetch a token is already in progress, don't bother with a new one.
    if (jwtTokenObtainingPromise != null) {
        return jwtTokenObtainingPromise;
    }

    jwtTokenObtainingPromise = new Promise((resolve, reject) => {
        $.get('/jwt/token-urls')
        .fail(error => {
            if (error.status == 404) {
                // This indicates that the Flamenco Manager has disabled security.
                resolve();
                return;
            }
            let event = new Event("JWTTokenManagerError");
            event.error = error;
            window.dispatchEvent(event);

            reject(error);
        })
        .done(tokenInfo => {
            // console.log("Token info:", tokenInfo)
            return $.ajax({
                method: 'GET',
                url: tokenInfo.tokenURL,
                xhrFields: {
                    withCredentials: true
                }
            })
            .fail(error => {
                if (error.status == 403) {
                    window.location = tokenInfo.loginURL;
                    return;
                }

                let event = new Event("JWTTokenServerError");
                event.error = error;
                window.dispatchEvent(event);

                reject(error);
            })
            .done(tokenResponse => {
                // console.log("JWT token received: ", tokenResponse);
                setJWTToken(tokenResponse);
                resolve();
            })
            ;
        })
        ;
    });

    jwtTokenObtainingPromise.finally(() => {
        jwtTokenObtainingPromise = null;
    });

    return jwtTokenObtainingPromise;
}