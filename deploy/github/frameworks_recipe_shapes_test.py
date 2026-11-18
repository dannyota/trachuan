import copy
import importlib
import unittest

from frameworks_test_support import release_recipes, vector_values


class RecipeShapesTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.import_module('frameworks_recipe_shapes')
        self.recipes = release_recipes()

    def refused(self, change):
        recipes = copy.deepcopy(self.recipes)
        change(recipes)
        with self.assertRaises(ValueError) as caught:
            self.module.check_recipe_shapes(recipes)
        self.assertNotIn('SENTINEL', str(caught.exception))

    def test_recipes_built_by_go_pass(self):
        self.module.check_recipe_shapes(self.recipes)

    def test_key_sets_match_the_go_recipes_in_the_vectors(self):
        parameters = vector_values()['step_build']['parameters']
        dense = parameters['dense_recipe']
        self.assertEqual(set(self.module.DENSE_1), set(dense))
        self.assertEqual(set(self.module.EMBEDDING), set(dense['document']))
        self.assertEqual(set(self.module.EMBEDDING), set(dense['query']))
        self.assertEqual(set(self.module.LEXICAL_1), set(parameters['lexical_recipe']))
        self.assertEqual(set(self.module.RETRIEVAL_2), set(parameters['retrieval_recipe']))
        self.assertEqual(set(self.module.CHUNK_1['parameters']), set(vector_values()['step_chunk']['parameters']))

    def test_unknown_or_missing_keys_refuse_at_every_level(self):
        paths = [('dense',), ('dense', 'document'), ('dense', 'query'), ('chunk',), ('chunk', 'parameters'), ('lexical',),
                 ('retrieval',), ('retrieval', 'rrf'), ('retrieval', 'abstention'), ('retrieval', 'source')]
        for path in paths:
            def target(recipes):
                value = recipes
                for key in path:
                    value = value[key]
                return value
            with self.subTest(path=path, change='extra'):
                self.refused(lambda recipes: target(recipes).update(note='LICENSED-SENTINEL'))
            with self.subTest(path=path, change='missing'):
                self.refused(lambda recipes: target(recipes).pop(next(iter(target(recipes)))))

    def test_unknown_versions_refuse(self):
        cases = [(name, version) for name, version in (('dense', 2), ('chunk', 2), ('lexical', 2), ('retrieval', 1), ('retrieval', 3))]
        cases += [('dense', value) for value in ('1', 1.0, True, None)]
        for name, version in cases:
            recipes = copy.deepcopy(self.recipes)
            recipes[name]['version'] = version
            with self.subTest(name=name, version=version), self.assertRaisesRegex(ValueError, 'unknown frameworks recipe version'):
                self.module.check_recipe_shapes(recipes)

    def test_value_types_refuse(self):
        changes = [lambda r: r['dense'].update(model=['list']), lambda r: r['dense'].update(dimensions='1024'),
                   lambda r: r['dense'].update(dimensions=True), lambda r: r['dense'].update(query_hash='x'),
                   lambda r: r['dense'].update(document_hash='A' * 64), lambda r: r['dense']['query'].update(truncation='no'),
                   lambda r: r['dense']['query'].update(token_cap=1.5), lambda r: r['dense']['document'].update(weights_sha256='x'),
                   lambda r: r['dense']['document'].update(resolved_revision='x'), lambda r: r['dense']['query'].update(model='x' * 201),
                   lambda r: r['dense']['query'].update(model='line\nbreak'), lambda r: r['chunk'].update(name='parse'),
                   lambda r: r['chunk']['parameters'].update(schema='chunks/2'), lambda r: r['chunk']['parameters'].update(limit='512'),
                   lambda r: r['lexical'].update(k1='1.2'), lambda r: r['lexical'].update(b=True), lambda r: r['lexical'].update(k1=float('inf')),
                   lambda r: r['retrieval'].update(dense_depth=1.5), lambda r: r['retrieval']['rrf'].update(k='60'),
                   lambda r: r['retrieval']['abstention'].update(threshold='0.5'), lambda r: r['retrieval']['abstention'].update(calibration_sha256='x'),
                   lambda r: r['retrieval']['source'].update(recipe_version='3')]
        for index, change in enumerate(changes):
            with self.subTest(index=index):
                self.refused(change)

    def test_only_a_measured_retrieval_recipe_can_be_signed(self):
        placeholder = vector_values()['step_build']['parameters']['retrieval_recipe']
        self.refused(lambda recipes: recipes.update(retrieval=placeholder))
        self.refused(lambda recipes: recipes['retrieval'].update(status='placeholder'))
        self.refused(lambda recipes: recipes['retrieval']['abstention'].update(status='bootstrap-from-law'))

    def test_the_recipe_set_is_closed(self):
        self.refused(lambda recipes: recipes.pop('chunk'))
        self.refused(lambda recipes: recipes.update(extra={'version': 1}))
        self.refused(lambda recipes: recipes.update(chunk=[]))


if __name__ == '__main__':
    unittest.main()
