# Ex.
#   GOVERSION=$(go env GOVERSION) GOARCH=$(go env GOARCH) vagrant up
#   vagrant rsync
#   vagrant ssh -c "cd wazero; go test ./..."

Vagrant.configure("2") do |config|
  config.vm.box = "generic/freebsd13"
  config.vm.synced_folder ".", "/home/vagrant/wazero",
    type: "rsync",
    rsync__exclude: ".git/"

  config.vm.provider "virtualbox" do |v|
    v.memory = 1024
    v.cpus = 1
  end

  # Ex. `GOVERSION=$(go env GOVERSION) GOARCH=$(go env GOARCH) vagrant provision`
  config.vm.provision "install-golang", type: "shell", run: "once" do |sh|
    sh.env = {
      'GOVERSION': ENV['GOVERSION'],
      'GOARCH': ENV['GOARCH'],
    }
    sh.inline = <<~GOINSTALL
      set -eux -o pipefail
      curl -fsSL "https://dl.google.com/go/${GOVERSION}.freebsd-${GOARCH}.tar.gz" | tar Cxz /usr/local
      cat >> /usr/local/etc/profile <<EOF
export GOROOT=/usr/local/go
export PATH=/usr/local/go/bin:$PATH
EOF
    GOINSTALL
  end
end


