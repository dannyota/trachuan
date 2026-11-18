import re

from .deploy_corpus_verify import toc as law_toc


NAME = '[a-z_][a-z0-9_]*'
VIEW = re.compile('CREATE OR REPLACE VIEW (' + NAME + ') AS')
FUNCTION = re.compile('CREATE OR REPLACE FUNCTION (' + NAME + r')\(\) RETURNS trigger\b.*')
TRIGGER = re.compile('CREATE OR REPLACE TRIGGER (' + NAME + r')\s+(?:BEFORE|AFTER|INSTEAD OF)\s+[A-Za-z_, ]*?\bON ('
                     + NAME + r')\s+FOR EACH (?:ROW|STATEMENT)\b')
DESCS = tuple(sorted(('SCHEMA', 'TABLE', 'TABLE DATA', 'TABLE ATTACH', 'SEQUENCE', 'SEQUENCE SET', 'SEQUENCE OWNED BY',
                      'DEFAULT', 'CONSTRAINT', 'FK CONSTRAINT', 'INDEX', 'INDEX ATTACH', 'STATISTICS', 'STATISTICS DATA',
                      'COMMENT', 'ACL', 'DEFAULT ACL', 'ENCODING', 'STDSTRINGS', 'SEARCHPATH', 'VIEW', 'FUNCTION',
                      'TRIGGER'), key=len, reverse=True))
ENTRIES = {'VIEW': re.compile('VIEW public (' + NAME + ') [^ ]*'),
           'FUNCTION': re.compile('FUNCTION public (' + NAME + r')\(\) [^ ]*'),
           'TRIGGER': re.compile('TRIGGER public (' + NAME + ') (' + NAME + ') [^ ]*')}


def objects(sql):
    found = set()
    for line in re.finditer(r'^CREATE\b.*$', sql, re.M):
        if view := VIEW.fullmatch(line[0]):
            found.add(('VIEW', view[1]))
        elif function := FUNCTION.fullmatch(line[0]):
            found.add(('FUNCTION', function[1]))
        elif trigger := TRIGGER.match(sql, line.start()):
            found.add(('TRIGGER', trigger[2], trigger[1]))
        else:
            raise ValueError('unrecognized frameworks schema object')
    return frozenset(found)


def namespace(entry):
    desc = next((desc for desc in DESCS if entry.startswith(desc + ' ')), None)
    return desc, (entry[len(desc) + 1:].split(' ', 1)[0] if desc else None)


def toc(text, extensions, sql):
    defined = objects(sql)

    def omit(entry):
        desc, space = namespace(entry)
        if desc == 'SCHEMA':
            if not re.fullmatch('SCHEMA - public [^ ]*', entry):
                raise ValueError('frameworks dump schema other than public')
            return False
        if space not in (None, '-', 'public'):
            raise ValueError('frameworks dump ' + desc.lower() + ' outside public schema')
        if desc not in ENTRIES:
            return False
        found = ENTRIES[desc].fullmatch(entry)
        if not found or (desc, *found.groups()) not in defined:
            raise ValueError('frameworks dump ' + desc.lower() + ' outside schema version')
        return True

    return law_toc(text, extensions, omit)
