#!/bin/bash -e

# Deploys onto biflamanager
DEPLOYHOST=biflamanager
DEPLOYPATH=/home/flamanager/  # end in slash!
SSH="ssh -o ClearAllForwardings=yes"

#echo "======== Building a statically-linked Flamenco Manager"
#rm -f flamenco-manager
#bash docker/build-via-docker.sh

echo "======== Deploying onto $DEPLOYHOST"
$SSH $DEPLOYHOST -t "sudo systemctl stop flamenco-manager.service"
rsync -e "$SSH" -va flamenco-manager $DEPLOYHOST:$DEPLOYPATH --delete-after
rsync -e "$SSH" -va templates static $DEPLOYHOST:$DEPLOYPATH --delete-after
$SSH $DEPLOYHOST -t "sudo systemctl start flamenco-manager.service"

echo "======== Deploy done."
