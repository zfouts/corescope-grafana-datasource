import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { VariableQueryEditor } from './VariableQueryEditor';

it('switches between observers and regions', () => {
  const onChange = jest.fn();
  render(<VariableQueryEditor query={{ kind: 'observers' }} onChange={onChange} />);
  fireEvent.click(screen.getByLabelText('Regions'));
  expect(onChange).toHaveBeenCalledWith({ kind: 'regions' }, 'Regions');
});
