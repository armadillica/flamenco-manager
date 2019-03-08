# Shaman

Shaman is a file storage server. It accepts uploaded files via HTTP, and stores them based on their
SHA256-sum and their file length. It can recreate directory structures by symlinking those files.
Shaman is intended to complement [Blender Asset
Tracer (BAT)](https://developer.blender.org/source/blender-asset-tracer/) and
[Flamenco](https://flamenco.io/), but can be used as a standalone component.

The overall use looks like this:

- User creates a set of files (generally via BAT-packing).
- User creates a Checkout Definition File (CDF), consisting of the SHA256-sums, file sizes, and file
  paths.
- User sends the CDF to Shaman for inspection.
- Shaman replies which files still need uploading.
- User sends those files.
- User sends the CDF to Shaman and requests a checkout with a certain ID.
- Shaman creates the checkout by symlinking the files listed in the CDF.
- Shaman responds with the directory the checkout was created in.

After this process, the checkout directory contains symlinks to all the files in the Checkout
Definition File. **The user only had to upload new and changed files.**


## File Store Structure

The Shaman file store is structured as follows:

    shaman-store/
        .. uploading/
            .. /{checksum[0:2]}/{checksum[2:]}/{filesize}-{unique-suffix}.tmp
        .. stored/
            .. /{checksum[0:2]}/{checksum[2:]}/{filesize}.blob

When a file is uploaded, it goes through several stages:

- Uploading: the file is being streamed over HTTP and in the process of
  being stored to disk. The `{checksum}` and `{filesize}` fields are
  as given by the user. While the file is being streamed to disk the
  SHA256 hash is calculated. After upload is complete the user-provided
  checksum and file size are compared to the SHA256 hash and actual size.
  If these differ, the file is rejected.
- Stored: after uploading is complete, the file is stored in the `stored`
  directory. Here the `{checksum}` and `{filesize}` fields can be assumed
  to be correct.

## Garbage Collection

To prevent infinite growth of the File Store, the Shaman will periodically
perform a garbage collection sweep. Garbage Collection can be configured by
setting the following settings in `shaman.yaml`:

- `garbageCollect.period`: this is the sleep time between garbage collector
  sweeps. Default is `8h`. Set to `0` to disable garbage collection.
- `garbageCollect.maxAge`: files that are newer than this age are not
  considered for garbage collection. Default is `744h` or 31 days.
- `garbageCollect.extraCheckoutPaths`: list of directories to include when
  searching for symlinks. Shaman will never create a checkout here.
  Default is empty.

Every time a file is symlinked into a checkout directory, it is 'touched'
(that is, its modification time is set to 'now').

Files that are not referenced in any checkout, and that have a modification
time that is older than `garbageCollectMaxAge` will be deleted.

To perform a dry run of the garbage collector, use `shaman -gc`.


## Key file generation

SHAman uses JWT with `ES256` signatures. The public keys of the JWT-signing
authority need to be known, and stored in `jwtkeys/*-public*.pem`.
For more info, see `jwtkeys/README.md`


## Source code structure

- `Makefile`: Used for building Shaman, testing, etc.
- `main.go`: The main entry point of the Shaman server. Handles CLI arguments,
  setting up logging, starting & stopping the server.
- `auth`: JWT token handling, authentication wrappers for HTTP handlers.
- `checkout`: Creates (and deletes) checkouts of files by creating directories
  and symlinking to the file storage.
- `config`: Configuration file handling.
- `fileserver`: Stores uploaded files in the file store, and serves files from
  it.
- `filestore`: Stores files by SHA256-sum and file size. Has separate storage
  bins for currently-uploading files and fully-stored files.
- `hasher`: Computes SHA256 sums.
- `httpserver`: The HTTP server itself (other packages just contain request
  handlers, and not the actual server).
- `libshaman`: Combines the other modules into one Shaman server struct.
  This allows `main.go` to start the Shaman server, and makes it possible in
  the future to embed a Shaman server into another Go project.
`_py_client`: An example client in Python. Just hacked together as a proof of
  concept and by no means of any official status.


## Non-source directories

- `jwtkeys`: Public keys + a private key for JWT sigining. For now Shaman can
  create its own dummy JWT keys, but in the future this will become optional
  or be removed altogether.
- `static`: For serving static files for the web interface.
- `views`: Contains HTML files for the web interface. This probably will be
  merged with `static` at some point.
