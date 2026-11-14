class Activator:
    def __init__(self, state, store, host):
        self.state, self.store, self.host = state, store, host

    def step(self, name):
        self.state.data['journal']['step'] = name
        self.state.save()

    def apply(self, candidate, restart, prefix=''):
        self.step(prefix + 'install')
        bundle_hash = candidate.manifest['host_bundle']['sha256']
        if self.state.data['installed_bundle'] != bundle_hash:
            self.state.data['installed_bundle'] = None
            self.state.save()
            self.host.install(self.store.unpack(candidate))
            self.state.data['installed_bundle'] = bundle_hash
            self.state.save()
        self.step(prefix + 'configure')
        self.host.configure(candidate)
        self.step(prefix + 'stop')
        self.host.systemctl('stop', 'caddy', 'web', 'server', 'migrate', 'queryembed')
        self.step(prefix + 'postgres')
        if restart:
            self.host.postgres()
        self.step(prefix + 'migrate')
        self.host.migrate()
        self.step(prefix + 'services')
        self.host.systemctl('start', 'server', 'web')
        self.step(prefix + 'loopback')
        self.host.probe()
        self.step(prefix + 'edge')
        self.host.clear_hold()
        self.host.systemctl('start', 'caddy')
        self.host.probe(edge=True)

    def activate(self, candidate, expected_schema=None, authorization=None, rollback=None):
        previous = self.state.data['active']
        baseline = self.host.schema()
        if expected_schema is not None and baseline != expected_schema:
            raise ValueError('rollback schema changed before activation')
        restart = self.host.postgres_changed(candidate)
        self.store.retain(candidate)
        self.state.data['retained'] = [previous] + [h for h in self.state.data['retained'] if h != previous]
        self.state.data['journal'] = dict(target=candidate.hash, previous=previous, baseline=baseline,
                                           restart_postgres=restart, status='started', step='started',
                                           authorization=authorization)
        if authorization:
            self.state.authorize(authorization, rollback, baseline)
        else:
            self.state.save()
        try:
            self.apply(candidate, restart)
            schema = self.host.schema()
            self.state.data['journal'].update(status='activated', step='complete')
            self.state.activate(candidate.hash, schema, authorization=authorization)
            self.store.prune(self.state)
        except BaseException:
            self.host.hold()
            raise

    def enforce_stopped(self):
        journal = self.state.data['journal']
        if journal and journal['status'] == 'stopped':
            self.host.hold()
            self.host.systemctl('stop', 'caddy')
            return True
        return False

    def recover(self):
        journal = self.state.data['journal']
        if self.enforce_stopped():
            return None
        if not journal or journal['status'] not in ('started', 'recovering'):
            return None
        self.host.hold()
        self.host.systemctl('stop', 'caddy')
        self.state.mark_failed(journal['target'])
        journal['status'] = 'recovering'
        self.state.save()
        try:
            self.host.systemctl('stop', 'web', 'server', 'migrate', 'queryembed')
            if journal['previous'] not in self.state.data['retained']:
                raise ValueError('previous manifest not retained')
            candidate = self.store.load(journal['previous'])
            if journal['restart_postgres']:
                self.host.restore_postgres(candidate, self.store.unpack(candidate))
            if self.host.schema() != journal['baseline']:
                raise ValueError('recovery schema changed or previous manifest not retained')
            restart = journal['restart_postgres'] or self.host.postgres_changed(candidate)
            self.apply(candidate, restart, 'rollback:')
            journal.update(status='rolled_back', step='complete')
            self.state.activate(candidate.hash, journal['baseline'])
            self.store.prune(self.state)
            return 'rolled_back'
        except BaseException as error:
            self.host.hold()
            if not isinstance(error, Exception):
                raise
            self.host.systemctl('stop', 'caddy')
            journal.update(status='stopped', step='recovery')
            self.state.save()
            return 'stopped'
