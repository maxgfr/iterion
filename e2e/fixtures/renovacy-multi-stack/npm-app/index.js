// Minimal consumer code so the upgrade pipeline has real call-sites to
// realign on breaking changes. Not production code.

const axios = require('axios');
const _ = require('lodash');

async function fetchTitle(url) {
  const res = await axios.get(url, { timeout: 5000 });
  return _.get(res, 'data.title', '');
}

module.exports = { fetchTitle };
