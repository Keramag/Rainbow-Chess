// Tests for the pure variant-picker helpers. Run with `node --test`.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  parseVariants,
  variantLabel,
  buildVariantOptions,
} from './variants.js';

test('parseVariants extracts the variant list from a welcome message', () => {
  const welcome = { type: 'welcome', variants: ['standard', 'rainbow'] };
  assert.deepEqual(parseVariants(welcome), ['standard', 'rainbow']);
});

test('parseVariants preserves server order', () => {
  assert.deepEqual(parseVariants({ variants: ['rainbow', 'standard'] }), [
    'rainbow',
    'standard',
  ]);
});

test('parseVariants tolerates missing / malformed payloads', () => {
  assert.deepEqual(parseVariants(undefined), []);
  assert.deepEqual(parseVariants(null), []);
  assert.deepEqual(parseVariants({}), []);
  assert.deepEqual(parseVariants({ variants: 'standard' }), []);
  assert.deepEqual(parseVariants({ variants: null }), []);
});

test('parseVariants drops non-string and empty entries', () => {
  const welcome = { variants: ['standard', '', 42, null, 'rainbow', undefined] };
  assert.deepEqual(parseVariants(welcome), ['standard', 'rainbow']);
});

test('variantLabel capitalises the registry key', () => {
  assert.equal(variantLabel('standard'), 'Standard');
  assert.equal(variantLabel('rainbow'), 'Rainbow');
});

test('variantLabel handles empty / non-string input', () => {
  assert.equal(variantLabel(''), '');
  assert.equal(variantLabel(undefined), '');
  assert.equal(variantLabel(123), '');
});

test('buildVariantOptions maps names to {value,label} pairs', () => {
  assert.deepEqual(buildVariantOptions(['standard', 'rainbow']), [
    { value: 'standard', label: 'Standard' },
    { value: 'rainbow', label: 'Rainbow' },
  ]);
});

test('buildVariantOptions sanitises just like parseVariants', () => {
  assert.deepEqual(buildVariantOptions(['standard', '', null, 'rainbow']), [
    { value: 'standard', label: 'Standard' },
    { value: 'rainbow', label: 'Rainbow' },
  ]);
  assert.deepEqual(buildVariantOptions(undefined), []);
});
