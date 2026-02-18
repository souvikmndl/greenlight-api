#!/bin/bash
set -eu

# ======================================================= #
# VARIABLES 
# ======================================================= #

TIMEZONE=Europe/Berlin
USERNAME=greenlight

# prompt to enter a password for the PostgreSQL greenlightuser
read -p "Enter the password for greenlight DB user: " DB_PASSWORD

# Force all output to be presented in en_US for the duration of this script
# This avoids anu "setting locale failed" errors while this script is running
export LC_ALL=en_US.UTF-8

# ======================================================= #
# SCRIPT LOGIC
# ======================================================= #

# enable the universe repository
add-apt-repository --yes universe

# update all software packages
apt update 

# set the system timezone and install all locales
timedatectl set-timezone ${TIMEZONE}
apt --yes install locales-all

# add the new user (and give them sudo privileges)
useradd --create-home --shell "/bin/bash" --group sudo "${USERNAME}"

# force a password to be set for the new user the first time they login
passwd --delete "${USERNAME}"
chage --lastday 0 "${USERNAME}"

# copy ssh keys from the root user to the new user
rsync --archive --chown=${USERNAME}:${USERNAME} /root/.ssh /home/${USERNAME}

# configure the firewall to allow SSH, HTTP and HTTPS traffice
ufw allow 22
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable

# install fail2ban
apt --yes install fail2ban

# install the migrate cli tool
curl -L https://github.com/golang-migrate/migrate/releases/download/v4.14.1/migrate.linux-amd64.tar.gz | tar xvz
mv migrate.linux-amd64 /usr/local/bin/migrate

# install postgres
apt --yes install postgresql

# Set up the greenlight DB and create a user account with the password entered earlier.
sudo -i -u postgres psql -c "CREATE DATABASE greenlight"
sudo -i -u postgres psql -d greenlight -c "CREATE EXTENSION IF NOT EXISTS citext"
sudo -i -u postgres psql -d greenlight -c "CREATE ROLE greenlight WITH LOGIN PASSWORD '${DB_PASSWORD}'"

# Add a DSN for connecting to the greenlight database to the system-wide environment
# variables in the /etc/environment file.
echo "GREENLIGHT_DB_DSN='postgres://greenlight:${DB_PASSWORD}@localhost/greenlight'" >> /etc/environment

# install caddy
apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-arc
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
apt update
apt --yes install caddy

# Upgrade all packages. Using the --force-confnew flag means that configuration
# files will be replaced if newer ones are available.
apt --yes -o Dpkg::Options::="--force-confnew" upgrade

echo "Script complete! Rebooting..."
reboot
