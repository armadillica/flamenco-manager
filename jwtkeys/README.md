# JWT Key Store

This directory contains the ECDSA keys for JWT signing and validation.

All files that match the `*-public*.pem` pattern will be loaded as public
keys, and be implicitly trusted as authoritative for any received JWT
token.

As an exception to the above rule, any key that has `test` in its filename
will be excluded. This is to prevent unit test keys from being loaded.


## Generating keys

One private key `es256-private.pem` may exist. If it does it will be
loaded to allow token generation for testing purposes. Note that
your Flamenco Manager WILL BE INSECURE as anyone accessing it can also
request an authentication token.

To generate a keypair for `ES256`:

    openssl ecparam -genkey -name prime256v1 -noout -out es256-private.pem
    openssl ec -in es256-private.pem -pubout -out es256-public.pem
