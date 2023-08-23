FROM alpine

RUN apk add wireguard-tools
RUN mkdir -p /etc/nettica
COPY nettica-client /usr/bin
CMD ["/usr/bin/nettica-client"]


