# JWT Key Store

This directory contains the ECDSA keys for JWT signing and validation.

One private key may exist and it should be named `es256-private.pem`.
If it exists it will be loaded to allow token generation for testing
purposes. Currently the key MUST exist, but that's because the software
isn't finished yet.

All files that match the `*-public*.pem` glob will be loaded as public
keys, and be implicitly trusted as authoritative for any received JWT
token. As an exception to this rule, any key that matches the above
glob pattern but also has `test` in its filename will be excluded. This
is to prevent unit test keys from being loaded.

To generate a keypair for `ES256`:

    openssl ecparam -genkey -name prime256v1 -noout -out es256-private.pem
    openssl ec -in es256-private.pem -pubout -out es256-public.pem
