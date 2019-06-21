/**
 * Copyright (c) 2017-present, Facebook, Inc.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// See https://docusaurus.io/docs/site-config for all the possible
// site configuration options.

// List of projects/orgs using your project for the users page.
const users = [
    {
        caption: 'Perlin',
        image: '/wavelet/img/icon-cumulus.svg',
        infoLink: 'https://perlin.net/cloudify',
        pinned: true,
    },
    {
        caption: 'Clarify',
        // You will need to prepend the image path with your baseUrl
        // if it is not '/', like: '/test-site/img/image.jpg'.
        image: '/wavelet/img/icon-clarify.svg',
        infoLink: 'https://perlin.net/clarify',
        pinned: true,
    },
    {
        caption: 'Certify',
        image: '/wavelet/img/icon-certify.svg',
        infoLink: 'https://perlin.net/certify',
        pinned: true,
    }
];

const siteConfig = {
    title: 'Wavelet', // Title for your website.
    tagline: 'An open ledger for writing scalable, mission-critical, decentralized WebAssembly applications in Go.',

    url: 'https://perlin-network.github.io',
    baseUrl: "/wavelet/",
    projectName: 'wavelet',
    organizationName: 'perlin-network',
    // For top-level user or org sites, the organization is still the same.
    // e.g., for the https://JoelMarcey.github.io site, it would be set like...
    //   organizationName: 'JoelMarcey'

    // For no header links in the top nav bar -> headerLinks: [],
    headerLinks: [
        {doc: 'setup', label: 'Docs'},
        {page: 'help', label: 'Help'},
        {search: true},
    ],

    // If you have users set above, you add it here:
    users,

    /* path to images for header/footer */
    headerIcon: 'img/favicon.ico',
    footerIcon: 'img/favicon.ico',
    favicon: 'img/favicon.ico',

    /* Colors for website */
    colors: {
        primaryColor: '#000000',
        secondaryColor: '#604bbc',
    },

    /* Custom fonts for website */
    fonts: {
        myFont: [
            "Times New Roman",
            "Serif"
        ],
        myOtherFont: [
            "-apple-system",
            "system-ui"
        ]
    },

    repoUrl: 'https://github.com/perlin-network/wavelet',
    editUrl: 'https://github.com/perlin-network/wavelet/edit/master/site/docs/',

    // This copyright info is used in /core/Footer.js and blog RSS/Atom feeds.
    copyright: `Copyright © ${new Date().getFullYear()} Perlin`,

    highlight: {
        // Highlight.js theme to use for syntax highlighting in code blocks.
        theme: 'monokai',
    },

    // Add custom scripts here that would be placed in <script> tags.
    scripts: ['https://buttons.github.io/buttons.js'],

    // On page navigation for the current documentation page.
    onPageNav: 'separate',
    // No .html extensions for paths.
    cleanUrl: true,

    // Open Graph and Twitter card images.
    ogImage: 'img/undraw_online.svg',

    twitter: true,
    twitterImage: 'img/undraw_tweetstorm.svg',
    twitterUsername: "PerlinNetwork",

    // Show documentation's last update time.
    enableUpdateTime: true,

    markdownPlugins: [
        md => {
            var katex = require("katex");

            function renderKatex(source, displayMode) {
                return katex.renderToString(source, {displayMode: displayMode, throwOnError: false});
            }

            function parseBlockKatex(state, startLine, endLine) {
                var marker, len, params, nextLine, mem,
                    haveEndMarker = false,
                    pos = state.bMarks[startLine] + state.tShift[startLine],
                    max = state.eMarks[startLine];
                var dollar = 0x24;

                if (pos + 1 > max) {
                    return false;
                }

                marker = state.src.charCodeAt(pos);
                if (marker !== dollar) {
                    return false;
                }

                // scan marker length
                mem = pos;
                pos = state.skipChars(pos, marker);
                len = pos - mem;

                if (len != 2) {
                    return false;
                }

                // search end of block
                nextLine = startLine;

                for (; ;) {
                    ++nextLine;
                    if (nextLine >= endLine) {

                        // unclosed block should be autoclosed by end of document.
                        // also block seems to be autoclosed by end of parent
                        break;
                    }

                    pos = mem = state.bMarks[nextLine] + state.tShift[nextLine];
                    max = state.eMarks[nextLine];

                    if (pos < max && state.tShift[nextLine] < state.blkIndent) {

                        // non-empty line with negative indent should stop the list:
                        // - ```
                        //  test
                        break;
                    }

                    if (state.src.charCodeAt(pos) !== dollar) {
                        continue
                    }
                    ;

                    if (state.tShift[nextLine] - state.blkIndent >= 4) {

                        // closing fence should be indented less than 4 spaces
                        continue;
                    }

                    pos = state.skipChars(pos, marker);

                    // closing code fence must be at least as long as the opening one
                    if (pos - mem < len) {
                        continue;
                    }

                    // make sure tail has spaces only
                    pos = state.skipSpaces(pos);

                    if (pos < max) {
                        continue;
                    }

                    haveEndMarker = true;

                    // found!
                    break;
                }

                // If a fence has heading spaces, they should be removed from its inner block
                len = state.tShift[startLine];

                state.line = nextLine + (haveEndMarker ? 1 : 0);

                var content = state.getLines(startLine + 1, nextLine, len, true)
                    .replace(/[ \n]+/g, ' ')
                    .trim();

                state.tokens.push({
                    type: 'katex',
                    params: params,
                    content: content,
                    lines: [startLine, state.line],
                    level: state.level,
                    block: true
                });

                return true;
            }

            function parseInlineKatex(state, silent) {
                var dollar = 0x24;
                var pos = state.pos;
                var start = pos, max = state.posMax, marker, matchStart, matchEnd;

                if (state.src.charCodeAt(pos) !== dollar) {
                    return false;
                }
                ++pos;

                while (pos < max && state.src.charCodeAt(pos) === dollar) {
                    ++pos;
                }

                marker = state.src.slice(start, pos);
                if (marker.length > 2) {
                    return false;
                }

                matchStart = matchEnd = pos;

                while ((matchStart = state.src.indexOf('$', matchEnd)) !== -1) {
                    matchEnd = matchStart + 1;

                    while (matchEnd < max && state.src.charCodeAt(matchEnd) === dollar) {
                        ++matchEnd;
                    }

                    if (matchEnd - matchStart == marker.length) {
                        if (!silent) {
                            var content = state.src.slice(pos, matchStart)
                                .replace(/[ \n]+/g, ' ')
                                .trim();

                            state.push({
                                type: 'katex',
                                content: content,
                                block: marker.length > 1,
                                level: state.level
                            });
                        }

                        state.pos = matchEnd;
                        return true;
                    }
                }

                if (!silent) state.pending += marker;
                state.pos += marker.length;

                return true;
            }

            md.inline.ruler.push('katex', parseInlineKatex);
            md.block.ruler.push('katex', parseBlockKatex);
            md.renderer.rules.katex = function (tokens, idx) {
                return renderKatex(tokens[idx].content, tokens[idx].block);
            };
        }
    ]
};

module.exports = siteConfig;
