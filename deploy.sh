#!/bin/bash -e

# Deploys onto biflamanager
DEPLOYHOST=biflamanager
DEPLOYPATH=/home/flamanager/  # end in slash!
SSH="ssh -o ClearAllForwardings=yes"

echo "======== Building a statically-linked Flamenco Manager"
bash docker/build-via-docker.sh linux

echo "======== Deploying onto $DEPLOYHOST"
$SSH $DEPLOYHOST -t "sudo systemctl stop flamenco-manager.service"
rsync -e "$SSH" -va docker/flamenco-manager-linux $DEPLOYHOST:$DEPLOYPATH/flamenco-manager --delete-after
rsync -e "$SSH" -va templates static $DEPLOYHOST:$DEPLOYPATH --delete-after --exclude static/latest-image.jpg
$SSH $DEPLOYHOST -t "sudo systemctl start flamenco-manager.service"

echo "======== Deploy done."
