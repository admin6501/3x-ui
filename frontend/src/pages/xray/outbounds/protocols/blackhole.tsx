import { useTranslation } from 'react-i18next';
import { Form } from 'antd';
import OptionButtons from '@/components/form/OptionButtons';

export default function BlackholeFields() {
  const { t } = useTranslation();
  return (
    <Form.Item label={t('pages.xray.outboundForm.responseType')} name={['settings', 'type']}>
      <OptionButtons
        options={[
          { value: '', label: '(empty)' },
          { value: 'none', label: 'none' },
          { value: 'http', label: 'http' },
        ]}
      />
    </Form.Item>
  );
}
