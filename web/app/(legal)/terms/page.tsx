import type { Metadata } from "next";
import Link from "next/link";
import { PRIVACY_EMAIL } from "../contact";

export const metadata: Metadata = {
  title: "Terms of Service | Blink",
  description: "The terms that apply when you use Blink to manage social posts.",
};

export default function TermsPage() {
  return (
    <>
      <h1>Terms of Service</h1>
      <p>
        Blink is a service based in Lagos, Nigeria that lets you connect social accounts, upload
        media, and publish or schedule posts. These Terms apply when you create an account or use Blink.
      </p>

      <section>
        <h2>Your account</h2>
        <p>
          You must be at least 18 years old and able to enter into a contract. If you use Blink
          for an organisation, you must have authority to act for it. Provide accurate account
          information, keep your password secure, and contact us if you suspect unauthorised use.
        </p>
      </section>

      <section>
        <h2>Content and connected accounts</h2>
        <p>
          You keep ownership of your content. You give Blink permission to store, process and
          transmit it as needed to provide the publishing features you request. Connect only
          accounts you are authorised to manage and upload only content you have the right to use.
          You are responsible for checking the destination, caption, media and scheduled time before
          submitting a post.
        </p>
      </section>

      <section>
        <h2>YouTube Terms of Service</h2>
        <p>
          By using Blink, you agree to be bound by the{" "}
          <a href="https://www.youtube.com/t/terms">YouTube Terms of Service</a>.
        </p>
      </section>

      <section>
        <h2>Acceptable use</h2>
        <p>
          Do not use Blink to publish unlawful content, infringe rights, send spam, impersonate
          others, distribute malware, or gain unauthorised access to accounts or systems. Follow
          the rules of each social platform you connect.
        </p>
      </section>

      <section>
        <h2>Publishing and third-party services</h2>
        <p>
          Publishing depends on connected platforms and their permissions, limits and availability.
          Posts can be delayed, fail, or succeed on only some destinations. Check the status in
          Blink and on the destination platform when delivery matters. You can manage scheduled
          posts before publishing begins. Removing a record in Blink does not remove a post
          already published on a social platform.
        </p>
      </section>

      <section>
        <h2>Ending access</h2>
        <p>
          You may stop using Blink, disconnect your accounts, or contact us to request account
          closure. Blink may restrict or suspend access for a material breach of these Terms,
          to address a security risk, or where required by law. Account closure and data requests
          are handled as described in the <Link href="/privacy">Privacy Policy</Link>.
        </p>
      </section>

      <section>
        <h2>Changes and applicable law</h2>
        <p>
          We will update the version on this page when these Terms change and notify users of
          material changes. Nigerian law applies. These Terms preserve rights and remedies that
          applicable law makes mandatory.
        </p>
      </section>

      <section>
        <h2>Contact Blink</h2>
        <p>Lagos, Nigeria. Email <a href={"mailto:" + PRIVACY_EMAIL}>{PRIVACY_EMAIL}</a>.</p>
      </section>
    </>
  );
}
