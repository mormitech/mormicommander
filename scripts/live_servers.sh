#!/usr/bin/env bash
# Starts the real servers the live integration tests (board issue #106, live_*_test.go) run against, on 127.0.0.1 of
# a throwaway Linux machine (CI's "live" job; it needs sudo and changes the system: a user, a CA certificate, an Apache
# site). Never run it on your own computer.
#
#   vsftpd   FTP on 2121: password login, passive and active mode, explicit TLS (AUTH TLS) with a self-signed
#            certificate that is added to the system's trusted certificates, UTF-8 names, ASCII mode converted
#   OpenSSH  SFTP on 2222: password (PAM) and an ed25519 key, an ed25519 host key only
#   Apache   WebDAV (mod_dav, mod_dav_fs) on 8080 under /dav, basic authentication
#   Samba    SMB on 445: the share "media" for mcuser only, no guests (network neighbourhood, #148)
#   NFS      the export of the live folder to 127.0.0.1 (nfs-kernel-server; showmount lists it)
#
# All serve the same user (mcuser). The packages must be installed already (vsftpd openssh-server apache2
# apache2-utils openssl samba nfs-kernel-server). It waits until every server answers (restarting one that does not, once) and writes the
# MC_LIVE_* variables for the tests to <dir>/live.env.
#
# Usage: scripts/live_servers.sh <dir>     (<dir>: keys, configuration and logs; uploaded by CI on failure)
set -euo pipefail

dir=${1:?usage: live_servers.sh <dir>}
mkdir -p "$dir"
dir=$(cd "$dir" && pwd)
user=mcuser
pass='mc-live-Pa55'          # a test user on a throwaway machine
home=/home/$user
data=$home/live              # the folder the tests work in, for FTP and SFTP alike

# ---- the user ----
if ! id "$user" >/dev/null 2>&1; then
  sudo useradd -m -s /bin/bash "$user"
fi
echo "$user:$pass" | sudo chpasswd
sudo install -d -m 755 -o "$user" -g "$user" "$home" "$data"
sudo chmod 755 "$home"
rm -f "$dir/client" "$dir/client.pub"
ssh-keygen -q -t ed25519 -N '' -C mc-live-client -f "$dir/client"
sudo install -d -m 700 -o "$user" -g "$user" "$home/.ssh"
sudo install -m 600 -o "$user" -g "$user" "$dir/client.pub" "$home/.ssh/authorized_keys"

# ---- vsftpd: FTP and explicit FTPS on 2121 ----
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj /CN=127.0.0.1 \
  -addext 'subjectAltName=IP:127.0.0.1,DNS:localhost' -keyout "$dir/ftp-key.pem" -out "$dir/ftp-cert.pem" 2>/dev/null
sudo install -m 644 "$dir/ftp-cert.pem" /usr/local/share/ca-certificates/mc-live-ftp.crt
sudo update-ca-certificates >/dev/null
sudo chown root:root "$dir/ftp-key.pem" && sudo chmod 600 "$dir/ftp-key.pem"
sudo systemctl stop vsftpd 2>/dev/null || true    # the package's own instance on port 21
sudo mkdir -p /var/run/vsftpd/empty
cat > "$dir/vsftpd.conf" <<EOF
listen=YES
listen_ipv6=NO
listen_address=127.0.0.1
listen_port=2121
background=YES
anonymous_enable=NO
local_enable=YES
write_enable=YES
local_umask=022
dirmessage_enable=NO
connect_from_port_20=NO
pasv_enable=YES
pasv_min_port=30000
pasv_max_port=30100
port_enable=YES
utf8_filesystem=YES
ascii_upload_enable=YES
ascii_download_enable=YES
secure_chroot_dir=/var/run/vsftpd/empty
pam_service_name=vsftpd
xferlog_enable=YES
dual_log_enable=YES
log_ftp_protocol=YES
vsftpd_log_file=$dir/vsftpd.log
xferlog_file=$dir/xferlog
ssl_enable=YES
rsa_cert_file=$dir/ftp-cert.pem
rsa_private_key_file=$dir/ftp-key.pem
allow_anon_ssl=NO
force_local_logins_ssl=NO
force_local_data_ssl=NO
EOF
# vsftpd refuses a config file that root does not own ("500 OOPS: config file not owned by correct user")
sudo chown root:root "$dir/vsftpd.conf"
start_vsftpd() {
  sudo vsftpd "$dir/vsftpd.conf" || { local rc=$?; echo "vsftpd did not start (exit $rc)"; sudo tail -20 "$dir/vsftpd.log" 2>/dev/null || true; return "$rc"; }
}

# ---- OpenSSH: SFTP on 2222 ----
sudo rm -f "$dir/ssh_host_ed25519_key" "$dir/ssh_host_ed25519_key.pub"
sudo ssh-keygen -q -t ed25519 -N '' -C mc-live-host -f "$dir/ssh_host_ed25519_key"
sudo chmod 644 "$dir/ssh_host_ed25519_key.pub"
sudo mkdir -p /run/sshd
cat > "$dir/sshd_config" <<EOF
Port 2222
ListenAddress 127.0.0.1
HostKey $dir/ssh_host_ed25519_key
PidFile $dir/sshd.pid
AllowUsers $user
PermitRootLogin no
PasswordAuthentication yes
KbdInteractiveAuthentication no
PubkeyAuthentication yes
UsePAM yes
Subsystem sftp internal-sftp
LogLevel VERBOSE
EOF
sudo /usr/sbin/sshd -t -f "$dir/sshd_config"
start_sshd() { sudo /usr/sbin/sshd -f "$dir/sshd_config" -E "$dir/sshd.log"; }

# ---- Apache mod_dav: WebDAV on 8080 under /dav ----
sudo a2enmod -q dav dav_fs auth_basic authn_file >/dev/null
sudo htpasswd -cb /etc/apache2/mc-live.htpasswd "$user" "$pass" 2>/dev/null
sudo install -d -o www-data -g www-data /var/www/mc-live-dav
sudo tee /etc/apache2/sites-available/mc-live-dav.conf >/dev/null <<'EOF'
Listen 127.0.0.1:8080
<VirtualHost 127.0.0.1:8080>
  DavLockDB ${APACHE_LOCK_DIR}/mc-live-DAVLock
  Alias /dav /var/www/mc-live-dav
  <Directory /var/www/mc-live-dav>
    Dav On
    AuthType Basic
    AuthName "mc-live"
    AuthUserFile /etc/apache2/mc-live.htpasswd
    Require valid-user
  </Directory>
  ErrorLog ${APACHE_LOG_DIR}/mc-live-error.log
  CustomLog ${APACHE_LOG_DIR}/mc-live-access.log combined
</VirtualHost>
EOF
sudo a2ensite -q mc-live-dav >/dev/null
sudo apache2ctl configtest
start_apache() { sudo systemctl restart apache2; }

# ---- start and wait ----
# answers: the server on port $1 sends something that matches $2 within a few seconds ("" = the port accepts).
answers() {
  local port=$1 want=$2 got
  if [ -z "$want" ]; then
    timeout 3 bash -c "exec 3<>/dev/tcp/127.0.0.1/$port" 2>/dev/null
    return
  fi
  got=$(timeout 3 bash -c "exec 3<>/dev/tcp/127.0.0.1/$port; IFS= read -r -t 2 line <&3; printf %s \"\$line\"" 2>/dev/null || true)
  [[ "$got" == *"$want"* ]]
}
wait_for() { # name port banner start-function
  local name=$1 port=$2 want=$3 start=$4 try
  # (a start that fails is reported and waited out like a slow one, so the log shows why instead of the script
  # ending silently under set -e)
  $start || echo "$name: the start command failed"
  for try in $(seq 1 40); do
    if answers "$port" "$want"; then echo "$name answers on $port"; return 0; fi
    if [ "$try" = 20 ]; then echo "$name does not answer yet: starting it again"; $start || true; fi
    sleep 0.5
  done
  echo "$name did not start"; return 1
}
# ---- Samba: SMB on 445, one share for mcuser, guests refused ----
sudo systemctl stop smbd nmbd 2>/dev/null || true
sudo pkill -x smbd 2>/dev/null || true
mkdir -p "$dir/smb/lock" "$dir/smb/state" "$dir/smb/cache" "$dir/smb/private"
cat > "$dir/smb.conf" <<EOF
[global]
  server role = standalone server
  map to guest = never
  smb ports = 445
  server min protocol = SMB2
  disable netbios = yes
  log file = $dir/smbd.log
  pid directory = $dir/smb
  lock directory = $dir/smb/lock
  state directory = $dir/smb/state
  cache directory = $dir/smb/cache
  private dir = $dir/smb/private
[media]
  path = $data
  read only = no
  valid users = $user
EOF
printf '%s\n%s\n' "$pass" "$pass" | sudo smbpasswd -c "$dir/smb.conf" -s -a "$user" >/dev/null
start_smbd() { sudo smbd -D -s "$dir/smb.conf"; }

# ---- NFS: the live folder exported to 127.0.0.1 ----
echo "$data 127.0.0.1(ro,insecure,no_subtree_check)" | sudo tee /etc/exports >/dev/null
sudo systemctl restart nfs-kernel-server 2>/dev/null || sudo service nfs-kernel-server restart
sudo exportfs -ra

wait_for vsftpd 2121 '220' start_vsftpd
wait_for sshd 2222 'SSH-2.0' start_sshd
wait_for apache 8080 '' start_apache
wait_for smbd 445 '' start_smbd
showmount -e 127.0.0.1 | grep -q "$data" || { echo "the NFS export is not listed"; showmount -e 127.0.0.1 || true; exit 1; }
# the WebDAV share takes the login
curl -fsS -o /dev/null -u "$user:$pass" -X PROPFIND -H 'Depth: 0' http://127.0.0.1:8080/dav/ \
  || { echo "the WebDAV share does not take the login"; exit 1; }

cat > "$dir/live.env" <<EOF
MC_LIVE_REQUIRE=all
MC_LIVE_FTP_HOST=127.0.0.1
MC_LIVE_FTP_PORT=2121
MC_LIVE_FTP_USER=$user
MC_LIVE_FTP_PASS=$pass
MC_LIVE_FTP_DIR=$data
MC_LIVE_FTPS=1
MC_LIVE_FTP_ACTIVE=1
MC_LIVE_SFTP_HOST=127.0.0.1
MC_LIVE_SFTP_PORT=2222
MC_LIVE_SFTP_USER=$user
MC_LIVE_SFTP_PASS=$pass
MC_LIVE_SFTP_DIR=$data
MC_LIVE_SFTP_KEY=$dir/client
MC_LIVE_SFTP_HOSTKEY=$dir/ssh_host_ed25519_key.pub
MC_LIVE_DAV_URL=webdav://127.0.0.1:8080/dav
MC_LIVE_DAV_USER=$user
MC_LIVE_DAV_PASS=$pass
MC_LIVE_SMB_HOST=127.0.0.1
MC_LIVE_SMB_USER=$user
MC_LIVE_SMB_PASS=$pass
MC_LIVE_SMB_SHARE=media
MC_LIVE_NFS_HOST=127.0.0.1
MC_LIVE_NFS_EXPORT=$data
EOF
echo "servers are up; the tests' settings are in $dir/live.env"
