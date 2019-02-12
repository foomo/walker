import Document, { DocumentProps, Head, Main, NextScript } from 'next/document';
import React from 'react';
import { ServerStyleSheet } from 'styled-components';

export interface GlobusDocumentProps extends DocumentProps {
  // Inject style tags for styled components
  styleTags: React.ReactElement<{}>[];
}

class GlobusDocument extends Document<GlobusDocumentProps> {
  static getInitialProps = async ({ renderPage }) => {
    const sheet = new ServerStyleSheet();
    const page = renderPage(App => props => sheet.collectStyles(<App {...props} />));
    const styleTags = sheet.getStyleElement();
    return { ...page, styleTags };
  }

  render() {
    return (
      <html>
        <Head>
          {this.props.styleTags}
          <meta name="viewport" content="width=device-width, initial-scale=1" />
        </Head>

        <body>
          <Main />
          <NextScript />
        </body>
      </html>
    );
  }
}

export const nextGlobusDocument = GlobusDocument;

export default GlobusDocument;