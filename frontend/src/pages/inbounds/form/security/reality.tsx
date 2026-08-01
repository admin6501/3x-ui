import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Collapse,
  Descriptions,
  Divider,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
} from 'antd';
import { RadarChartOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';

import { UTLS_FINGERPRINT } from '@/schemas/primitives';
import {
  validateRealityClientVer,
  validateRealityMaxClientVer,
  validateRealityTarget,
} from '@/lib/xray/stream-wire-normalize';
import type { RealityScanResult } from '@/generated/types';
import RealityTargetScannerModal from './RealityTargetScannerModal';
import TagsInput from '@/components/form/TagsInput';

interface RealityFormProps {
  saving: boolean;
  randomizeRealityTarget: () => void;
  scanning: boolean;
  scanResult: RealityScanResult | null;
  scanRealityTarget: () => void;
  scanRealityCandidates: (targets?: string) => Promise<RealityScanResult[]>;
  applyRealityScanResult: (result: RealityScanResult) => void;
  randomizeShortIds: () => void;
  genRealityKeypair: () => void;
  clearRealityKeypair: () => void;
  genMldsa65: () => void;
  clearMldsa65: () => void;
}

export default function RealityForm({
  saving,
  randomizeRealityTarget,
  scanning,
  scanResult,
  scanRealityTarget,
  scanRealityCandidates,
  applyRealityScanResult,
  randomizeShortIds,
  genRealityKeypair,
  clearRealityKeypair,
  genMldsa65,
  clearMldsa65,
}: RealityFormProps) {
  const { t } = useTranslation();
  const [scannerOpen, setScannerOpen] = useState(false);
  return (
    <>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'show']}
        label={t('pages.inbounds.form.show')}
        valuePropName="checked"
      >
        <Switch />
      </Form.Item>
      <Form.Item name={['streamSettings', 'realitySettings', 'xver']} label={t('pages.inbounds.form.xver')}>
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'settings', 'fingerprint']}
        label="uTLS"
      >
        <Select
          options={Object.values(UTLS_FINGERPRINT).map((fp) => ({ value: fp, label: fp }))}
        />
      </Form.Item>
      <Form.Item
        label={t('pages.inbounds.form.target')}
        tooltip={t('pages.inbounds.form.realityTargetHint')}
      >
        <Space.Compact block style={{ display: 'flex' }}>
          <Form.Item
            name={['streamSettings', 'realitySettings', 'target']}
            noStyle
            rules={[
              {
                validator: async (_, value) => {
                  const errKey = validateRealityTarget(typeof value === 'string' ? value : '');
                  if (errKey) throw new Error(t(errKey));
                },
              },
            ]}
          >
            <Input style={{ flex: 1 }} placeholder="example.com:443" />
          </Form.Item>
          <Button
            aria-label={t('pages.inbounds.form.scan')}
            icon={<RadarChartOutlined />}
            loading={scanning}
            onClick={scanRealityTarget}
          />
          <Button
            aria-label={t('pages.inbounds.form.findTargets')}
            icon={<SearchOutlined />}
            onClick={() => setScannerOpen(true)}
          />
          <Button
            aria-label={t('regenerate')}
            icon={<ReloadOutlined />}
            onClick={randomizeRealityTarget}
          />
        </Space.Compact>
      </Form.Item>
      {scanResult && (
        <Form.Item label=" " colon={false}>
          <Alert
            type={scanResult.feasible ? 'success' : 'warning'}
            showIcon
            title={
              scanResult.feasible
                ? t('pages.inbounds.form.scanFeasible')
                : scanResult.reason || t('pages.inbounds.form.scanNotFeasible')
            }
            description={
              <Descriptions size="small" column={1}>
                <Descriptions.Item label="TLS">{scanResult.tlsVersion || '—'}</Descriptions.Item>
                <Descriptions.Item label="ALPN">{scanResult.alpn || '—'}</Descriptions.Item>
                <Descriptions.Item label={t('pages.inbounds.form.scanCurve')}>
                  {scanResult.curveID || '—'}
                </Descriptions.Item>
                <Descriptions.Item label={t('pages.inbounds.form.scanCert')}>
                  {scanResult.certValid
                    ? `${scanResult.certSubject} (${scanResult.certIssuer})`
                    : t('pages.inbounds.form.scanCertInvalid')}
                </Descriptions.Item>
                <Descriptions.Item label={t('pages.inbounds.form.scanLatency')}>
                  {scanResult.latencyMs > 0 ? `${scanResult.latencyMs} ms` : '—'}
                </Descriptions.Item>
              </Descriptions>
            }
          />
        </Form.Item>
      )}
      <Form.Item label="SNI">
        <Space.Compact block style={{ display: 'flex' }}>
          <Form.Item
            name={['streamSettings', 'realitySettings', 'serverNames']}
            noStyle
          >
            <TagsInput style={{ flex: 1 }} />
          </Form.Item>
          <Button icon={<ReloadOutlined />} onClick={randomizeRealityTarget} />
        </Space.Compact>
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'maxTimediff']}
        label={t('pages.inbounds.form.maxTimeDiff')}
      >
        <InputNumber min={0} />
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'minClientVer']}
        label={t('pages.inbounds.form.minClientVer')}
        tooltip={t('pages.inbounds.form.minClientVerHint')}
        rules={[
          {
            validator: async (_, value) => {
              const errKey = validateRealityClientVer(typeof value === 'string' ? value : '');
              if (errKey) throw new Error(t(errKey));
            },
          },
        ]}
      >
        <Input placeholder="26.3.27" />
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'maxClientVer']}
        label={t('pages.inbounds.form.maxClientVer')}
        tooltip={t('pages.inbounds.form.maxClientVerHint')}
        dependencies={[['streamSettings', 'realitySettings', 'minClientVer']]}
        rules={[
          ({ getFieldValue }) => ({
            validator: async (_, value) => {
              const min = getFieldValue(['streamSettings', 'realitySettings', 'minClientVer']);
              const errKey = validateRealityMaxClientVer(
                typeof value === 'string' ? value : '',
                typeof min === 'string' ? min : '',
              );
              if (errKey) throw new Error(t(errKey));
            },
          }),
        ]}
      >
        <Input placeholder="x.y.z" />
      </Form.Item>
      <Form.Item label={t('pages.inbounds.form.shortIds')}>
        <Space.Compact block style={{ display: 'flex' }}>
          <Form.Item
            name={['streamSettings', 'realitySettings', 'shortIds']}
            noStyle
          >
            <TagsInput style={{ flex: 1 }} />
          </Form.Item>
          <Button icon={<ReloadOutlined />} onClick={randomizeShortIds} />
        </Space.Compact>
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'settings', 'spiderX']}
        label={t('pages.inbounds.form.spiderX')}
      >
        <Input />
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'settings', 'publicKey']}
        label={t('pages.inbounds.publicKey')}
      >
        <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} />
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'privateKey']}
        label={t('pages.inbounds.privatekey')}
      >
        <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} />
      </Form.Item>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" loading={saving} onClick={genRealityKeypair}>
            {t('pages.inbounds.form.getNewCert')}
          </Button>
          <Button danger onClick={clearRealityKeypair}>{t('clear')}</Button>
        </Space>
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'mldsa65Seed']}
        label={t('pages.inbounds.form.mldsa65Seed')}
      >
        <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'settings', 'mldsa65Verify']}
        label={t('pages.inbounds.form.mldsa65Verify')}
      >
        <Input.TextArea autoSize={{ minRows: 2, maxRows: 6 }} />
      </Form.Item>
      <Form.Item label=" ">
        <Space>
          <Button type="primary" loading={saving} onClick={genMldsa65}>
            {t('pages.inbounds.form.getNewSeed')}
          </Button>
          <Button danger onClick={clearMldsa65}>{t('clear')}</Button>
        </Space>
      </Form.Item>
      <Form.Item
        name={['streamSettings', 'realitySettings', 'masterKeyLog']}
        label={t('pages.inbounds.form.masterKeyLog')}
        tooltip={t('pages.inbounds.form.masterKeyLogTip')}
      >
        <Input placeholder="/path/to/sslkeylog.txt" />
      </Form.Item>
      <RealityTargetScannerModal
        open={scannerOpen}
        onClose={() => setScannerOpen(false)}
        scanRealityCandidates={scanRealityCandidates}
        onPick={applyRealityScanResult}
      />
      <Collapse
        style={{ marginBottom: 14 }}
        items={[
          {
            key: 'limitFallback',
            label: t('pages.inbounds.form.limitFallback'),
            children: (
              <>
                {(['limitFallbackUpload', 'limitFallbackDownload'] as const).map((dir) => (
                  <div key={dir}>
                    <Divider style={{ margin: '0 0 14px 0' }}>
                      {t(`pages.inbounds.form.${dir}`)}
                    </Divider>
                    <Form.Item
                      name={['streamSettings', 'realitySettings', dir, 'afterBytes']}
                      label={t('pages.inbounds.form.afterBytes')}
                      tooltip={t('pages.inbounds.form.afterBytesTip')}
                    >
                      <InputNumber min={0} />
                    </Form.Item>
                    <Form.Item
                      name={['streamSettings', 'realitySettings', dir, 'bytesPerSec']}
                      label={t('pages.inbounds.form.bytesPerSec')}
                      tooltip={t('pages.inbounds.form.bytesPerSecTip')}
                    >
                      <InputNumber min={0} />
                    </Form.Item>
                    <Form.Item
                      name={['streamSettings', 'realitySettings', dir, 'burstBytesPerSec']}
                      label={t('pages.inbounds.form.burstBytesPerSec')}
                      tooltip={t('pages.inbounds.form.burstBytesPerSecTip')}
                    >
                      <InputNumber min={0} />
                    </Form.Item>
                  </div>
                ))}
              </>
            ),
          },
        ]}
      />
    </>
  );
}
