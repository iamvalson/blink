import type { Metadata } from "next";
import { PRIVACY_EMAIL } from "../contact";

export const metadata: Metadata = {
  title: "Privacy Policy | Blink",
  description: "How Blink handles personal data and how to exercise your privacy rights.",
};

export default function PrivacyPage() {
  return (
    <>
      <h1>Privacy Policy</h1>
      <p>
        Blink, based in Lagos, Nigeria, is responsible for personal data it processes to operate
        this service. For privacy questions or requests, email{" "}
        <a href={"mailto:" + PRIVACY_EMAIL}>{PRIVACY_EMAIL}</a>.
      </p>

      <section>
        <h2>What we collect, why, and the lawful basis</h2>
        <ul>
          <li>
            <strong>Accounts and recovery.</strong> We use your display name, email address,
            password hash and session information to create and secure your account, sign you in,
            and help you reset your password. This is necessary to provide the service you request
            under our agreement. A name, email and password are required to open an account.
          </li>
          <li>
            <strong>Publishing.</strong> When you connect a social account, the platform provides
            account identifiers, permissions and authorisation tokens. We use these with the
            captions, media, destinations and schedules you provide to carry out your publishing
            requests under our agreement. Connecting an account and uploading content are optional
            until you choose to use those features. Disconnecting an account in Blink revokes its saved
            authorization with the platform and removes the connection and its associated publishing
            records from Blink.
          </li>
          <li>
            <strong>Security and operation.</strong> We use technical request and job information
            to limit abuse, diagnose failures and keep the service reliable. Our lawful basis is
            our legitimate interest in protecting the service and its users, subject to your
            rights. We keep the version and time of signup acceptance to document the agreement
            and handle disputes. We also process data where a legal obligation requires it.
          </li>
        </ul>
        <p className="mt-3">
          Acknowledging this notice does not give Blink consent for unrelated uses. Where consent
          is the applicable basis, we ask for it separately and you may withdraw it at any time.
        </p>
      </section>

      <section>
        <h2>YouTube API Services</h2>
        <p>
          Blink uses YouTube API Services when you connect a YouTube channel. Blink reads your
          channel ID to identify the connected channel and stores it with your encrypted
          authorization tokens. When you publish or schedule a YouTube post, Blink sends the
          video and publishing details you provide to YouTube. Blink uses the resulting video
          ID and YouTube processing status to track delivery and show your publishing history.
          See the <a href="https://www.google.com/policies/privacy">Google Privacy Policy</a> for
          how Google handles your data.
        </p>
        <p className="mt-3">
          Disconnect YouTube on the Accounts page to revoke Blink&apos;s authorization with
          Google and remove the connection and its associated YouTube publishing records from
          Blink. You can also open your{" "}
          <a href="https://security.google.com/settings/security/permissions">Google Account permissions</a>
          {" "}and remove access for Blink by earthtovalentine. To request deletion of YouTube
          data held by Blink or ask a privacy question, email{" "}
          <a href={"mailto:" + PRIVACY_EMAIL}>{PRIVACY_EMAIL}</a>.
        </p>
      </section>

      <section>
        <h2>Who receives data</h2>
        <ul>
          <li>Hosting and infrastructure providers process the information needed to operate the app, database and publishing queues.</li>
          <li>Cloudflare R2 stores uploaded media when Blink uses its cloud storage configuration.</li>
          <li>Resend delivers password reset emails when you request one.</li>
          <li>X, Google/YouTube and TikTok receive the information needed for the connections and publishing actions you choose. Once content is published, the platform handles it under its own privacy rules and the visibility settings on that platform.</li>
          <li>We may disclose relevant data to a competent authority where the law requires it.</li>
        </ul>
      </section>

      <section>
        <h2>Processing outside Nigeria</h2>
        <p>
          Cloud service providers and connected social platforms may handle information outside
          Nigeria. Where a transfer is subject to the Nigeria Data Protection Act, Blink must
          establish an applicable transfer basis and safeguards before transferring the data.
          Contact us for information about destinations and safeguards that apply to your data.
        </p>
      </section>

      <section>
        <h2>How long we keep information</h2>
        <p>
          Account data is needed while your account remains open. We keep post and media records
          to display your publishing history and carry out pending requests. You can request
          deletion of your account or data by email. When information is no longer needed, we
          delete it unless we have a lawful reason to retain it, such as a legal obligation,
          an unresolved dispute, or security incident investigation. Password reset links
          expire after 30 minutes.
        </p>
      </section>

      <section>
        <h2>Cookies, security and automation</h2>
        <p>
          Blink uses an essential session cookie to keep you signed in. Logging out clears that
          cookie. Passwords are stored as hashes, and connected platform tokens are stored
          encrypted. Blink uses automation to deliver scheduled posts and retry certain failures.
          These publishing operations carry out your instructions. They are not used to profile
          you or make decisions about you with legal or similarly significant effects.
        </p>
      </section>

      <section>
        <h2>Your privacy rights</h2>
        <p>
          Under the Nigeria Data Protection Act, you can ask for access to and information about
          your personal data, correction, deletion, restriction, or a portable copy where
          applicable. You may object to processing, withdraw consent where we rely on it, and
          challenge qualifying solely automated decisions. Withdrawal does not affect the lawfulness
          of earlier processing.
        </p>
        <p className="mt-3">
          Email <a href={"mailto:" + PRIVACY_EMAIL}>{PRIVACY_EMAIL}</a> with your request.
          We may need to verify your identity before sharing or changing account data.
          You can also <a href="https://services.ndpc.gov.ng/breach/">lodge a complaint with the Nigeria Data Protection Commission</a>.
        </p>
      </section>

      <section>
        <h2>Children and changes to this notice</h2>
        <p>
          Blink is intended for adults. Contact us if you believe a child has provided personal
          data through Blink. We will update the version on this page when the notice changes
          and notify users of material changes.
        </p>
      </section>
    </>
  );
}
