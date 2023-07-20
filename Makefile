DIR := $(shell pwd)
SH := bash
PROD_IMAGE_NAME=nettica-client-build
USERID := $(shell id -u)
GROUPID := $(shell id -g)
export USERID GROUPID

VERSION=1.0.0

build:
	docker build .
	docker-compose up $(PROD_IMAGE_NAME)

build-deb:
	./debian/rules clean binary && \
	mv ../*.deb $(DIR)/. && \
	./debian/rules clean && \
	git checkout -- ./debian/changelog
	chown $(USERID):$(GROUPID) *.deb

clean:
	-docker-compose down --rmi all
	-rm -rf *.deb
