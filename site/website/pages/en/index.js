/**
 * Copyright (c) 2017-present, Facebook, Inc.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const React = require('react');

const CompLibrary = require('../../core/CompLibrary.js');

const MarkdownBlock = CompLibrary.MarkdownBlock; /* Used to read markdown */
const Container = CompLibrary.Container;
const GridBlock = CompLibrary.GridBlock;

class HomeSplash extends React.Component {
    render() {
        const {siteConfig, language = ''} = this.props;
        const {baseUrl, docsUrl} = siteConfig;
        const docsPart = `${docsUrl ? `${docsUrl}/` : ''}`;
        const langPart = `${language ? `${language}/` : ''}`;
        const docUrl = doc => `${baseUrl}${docsPart}${langPart}${doc}`;

        const SplashContainer = props => (
            <div className="homeContainer">
                <div className="homeSplashFade">
                    <div className="wrapper homeWrapper">{props.children}</div>
                </div>
            </div>
        );

        const ProjectTitle = () => (
            <h2 className="projectTitle">
                {siteConfig.title}
                <small>{siteConfig.tagline}</small>
            </h2>
        );

        const PromoSection = props => (
            <div className="section promoSection">
                <div className="promoRow">
                    <div className="pluginRowBlock">{props.children}</div>
                </div>
            </div>
        );

        const Button = props => (
            <div className="pluginWrapper buttonWrapper">
                <a className="button" href={props.href} target={props.target}>
                    {props.children}
                </a>
            </div>
        );

        return (
            <SplashContainer>
                <div className="inner">
                    <ProjectTitle siteConfig={siteConfig}/>
                    <PromoSection>
                        <Button href={`${docUrl('setup')}`}>Get Started</Button>
                        <Button href="/whitepaper.pdf">Whitepaper</Button>
                        <Button href="https://lens.perlin.net">Testnet</Button>
                        <Button href="https://github.com/perlin-network/wavelet">Github</Button>
                    </PromoSection>
                </div>
            </SplashContainer>
        );
    }
}

const smartContract = `
Miss using your IDE, debugging tools, and ability to write unit tests, benchmarks, and end-to-end pipelines when it comes to writing smart contracts?

Wavelet allows developers to write _mission-critical_, _robust_, and _scalable_ decentralized applications
in the form of [WebAssembly](https://webassembly.org/) smart contracts, with batteries initially included for development in [Rust](https://www.rust-lang.org/).

Use the same 'ole tools you know and love to efficiently and cost-effectively create your next decentralized application.

[Click here](/docs/smart-contracts) for more info about Wavelet's Rust smart contract SDK.

[Click here](https://github.com/perlin-network/smart-contract-as) for more info about Wavelet's AssemblyScript smart contract SDK.
`;

class Index extends React.Component {
    render() {
        const {config: siteConfig, language = ''} = this.props;
        const {baseUrl} = siteConfig;

        const Block = props => (
            <Container
                padding={['bottom', 'top']}
                id={props.id}
                background={props.background}>
                <GridBlock
                    align="left"
                    contents={props.children}
                    layout={props.layout}
                />
            </Container>
        );

        const FeatureCallout = () => (
            <div
                className="productShowcaseSection paddingBottom"
                style={{textAlign: 'center'}}>
                <h2>"The next generation of cloudless computing."</h2>
                <MarkdownBlock>Build disaster-proof applications with our community on [Discord](https://discord.gg/dMYfDPM).</MarkdownBlock>
            </div>
        );

        const SmartContracts = () => (
            <Block background="light">
                {[
                    {
                        content: smartContract,
                        image: `${baseUrl}img/ide.png`,
                        imageAlign: 'right',
                        title: 'Write once, run anywhere.',
                    },
                ]}
            </Block>
        );

        const Features = () => (
            <Block layout="fourColumn">
                {[
                    {
                        content: 'Wavelet has been _thoroughly_  benchmarked in being capable of processing over **31,240 payment transactions per second** using 240 [DigitalOcean](https://www.digitalocean.com/) instances (2vCPUs, 4GB RAM) under realistic networking conditions where there exists 2% packet loss, capped transfer rates of 1MB/s, and 220ms communication latency.',
                        image: `${baseUrl}img/icon-scalable.svg`,
                        imageAlign: "top",
                        title: 'Scalable.',
                    },
                    {
                        content: 'Wavelet guarantees that transactions are _totally ordered_, _replicated_, and _consistent_ on a network of untrusted machines, and thus supports _upgradeability_, _decentralized governance_, and _smart contract execution_ with **2-4 second finality on millions of nodes**.',
                        image: `${baseUrl}img/icon-time.svg`,
                        imageAlign: "top",
                        title: 'Practical.',
                    },
                    {
                        content: 'Wavelet guarantees that transactions are _immutable_, and _irrevertible_ once they are finalized. A novel, secure pruning mechanism reduces the system requirements for running a full Wavelet node to the point of only requiring a healthy Internet connection and **512MB of RAM**.',
                        image: `${baseUrl}img/icon-tps.svg`,
                        imageAlign: "top",
                        title: 'Succinct.',
                    },
                    {
                        content: 'No committees, no leaders, no central authority; **absolutely zero trust**. Wavelet is open, permissionless, and leaderless by introducing a novel, fair, and green voting protocol where nodes are incentivized through virtual rewards to constantly protect and validate the network.',
                        image: `${baseUrl}img/icon-green.svg`,
                        imageAlign: "top",
                        title: 'Secure.',
                    },

                ]}
            </Block>
        );

        const Showcase = () => {
            if ((siteConfig.users || []).length === 0) {
                return null;
            }

            const showcase = siteConfig.users
                .filter(user => user.pinned)
                .map(user => (
                    <a href={user.infoLink} key={user.infoLink}>
                        <img src={user.image} alt={user.caption} title={user.caption}/>
                        <p><b>{user.caption}</b></p>
                    </a>
                ));

            const pageUrl = page => baseUrl + (language ? `${language}/` : '') + page;

            return (
                <div className="productShowcaseSection paddingBottom">
                    <h2>Who is Using Wavelet?</h2>

                    <div className="logos">{showcase}</div>
                    <div className="more-users">
                        <a className="button" href={pageUrl('users.html')}>
                            More {siteConfig.title} Users
                        </a>
                    </div>
                </div>
            );
        };

        return (
            <div>
                <HomeSplash siteConfig={siteConfig} language={language}/>
                <div className="mainContainer">
                    <Features/>
                    <FeatureCallout/>
                    <SmartContracts/>
                    <Showcase/>
                </div>
            </div>
        );
    }
}

module.exports = Index;
