FROM golang:1.4
VOLUME ["/git"]
ENV GIT_SYNC_DEST /git
ADD ./git-sync /go/bin/git-sync
ENTRYPOINT ["/go/bin/git-sync"]
