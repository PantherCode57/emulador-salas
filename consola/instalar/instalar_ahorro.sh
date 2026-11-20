#!/bin/bash
# Ahorro de batería: el servicio panther-ahorro (consola/ahorro), como root, que arranca con la
# consola. Se lanza como ark con el programa compilado (panther-ahorro) y panther-ahorro.service en
# ~/panther-instalar.
set -e
I=~/panther-instalar
sudo install -m 755 $I/panther-ahorro /usr/local/bin/panther-ahorro
sudo install -m 644 $I/panther-ahorro.service /etc/systemd/system/panther-ahorro.service
sudo systemctl daemon-reload
sudo systemctl enable panther-ahorro.service
sudo systemctl restart panther-ahorro.service
systemctl is-active panther-ahorro.service
