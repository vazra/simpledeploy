---
title: Ansible
description: Provision and configure SimpleDeploy with Ansible. Community playbook stub.
---

There is no official Ansible collection yet. Below is a sketch a community playbook could follow. Contributions welcome at [github.com/vazra/simpledeploy](https://github.com/vazra/simpledeploy).

## What a playbook should do

1. Install Docker and the SimpleDeploy package on each target host.
2. Render `/etc/simpledeploy/config.yaml` from a template (per-host secrets).
3. Open ports 80 and 443 in the firewall.
4. Enable and start the systemd service.
5. Bootstrap the first admin user.
6. Place compose files into `apps_dir/<slug>/docker-compose.yml`. The reconciler picks them up.

## Sketch: one host, Ubuntu

```yaml
- hosts: vps
  become: true
  vars:
    sd_master_secret: "{{ vault_master_secret }}"
    sd_tls_email: ops@example.com
  tasks:
    - name: Install Docker
      apt:
        name: docker.io
        state: present
        update_cache: true

    - name: Add SimpleDeploy apt key
      ansible.builtin.get_url:
        url: https://vazra.github.io/apt-repo/gpg.key
        dest: /usr/share/keyrings/vazra.gpg

    - name: Add SimpleDeploy apt source
      ansible.builtin.copy:
        dest: /etc/apt/sources.list.d/vazra.list
        content: "deb [signed-by=/usr/share/keyrings/vazra.gpg] https://vazra.github.io/apt-repo stable main"

    - name: Install SimpleDeploy
      apt:
        name: simpledeploy
        state: present
        update_cache: true

    - name: Render config
      template:
        src: config.yaml.j2
        dest: /etc/simpledeploy/config.yaml
        owner: simpledeploy
        group: simpledeploy
        mode: "0600"
      notify: restart simpledeploy

    - name: Open firewall
      ufw:
        rule: allow
        port: "{{ item }}"
      loop: [80, 443]

    - name: Enable service
      systemd:
        name: simpledeploy
        enabled: true
        state: started

    - name: Drop a compose file for myapp
      copy:
        src: "apps/myapp/docker-compose.yml"
        dest: "/etc/simpledeploy/apps/myapp/docker-compose.yml"
        owner: simpledeploy
        group: simpledeploy
        mode: "0644"

  handlers:
    - name: restart simpledeploy
      systemd:
        name: simpledeploy
        state: restarted
```

Bootstrap the admin user once via `simpledeploy users create` (don't put the password in plaintext; pass via `SD_PASSWORD` env from a vault var).

## Want to publish a collection?

Open a PR adding a link here, or open an issue describing what your collection covers.
