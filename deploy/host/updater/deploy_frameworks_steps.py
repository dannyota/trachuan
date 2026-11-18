class Steps:
    def __init__(self, database):
        self.db = database

    def refuse(self, *args):
        raise ValueError('frameworks restore not implemented')

    inspect = create = restore = validate = smoke = refuse
