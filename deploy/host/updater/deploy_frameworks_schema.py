import re

from .deploy_corpus_verify import toc as law_toc


NAME = '[a-z_][a-z0-9_]*'
VIEW = re.compile('CREATE OR REPLACE VIEW (' + NAME + ') AS')
FUNCTION = re.compile('CREATE OR REPLACE FUNCTION (' + NAME + r')\(\) RETURNS trigger\b.*')
TRIGGER = re.compile('CREATE OR REPLACE TRIGGER (' + NAME + r')\s+(?:BEFORE|AFTER|INSTEAD OF)\s+[A-Za-z_, ]*?\bON ('
                     + NAME + r')\s+FOR EACH (?:ROW|STATEMENT)\b')
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


def toc(text, extensions, sql):
    defined = objects(sql)

    def omit(entry):
        kind = entry.split(' ', 1)[0]
        if kind not in ENTRIES:
            return False
        found = ENTRIES[kind].fullmatch(entry)
        if not found or (kind, *found.groups()) not in defined:
            raise ValueError('frameworks dump ' + kind.lower() + ' outside schema version')
        return True

    return law_toc(text, extensions, omit)
